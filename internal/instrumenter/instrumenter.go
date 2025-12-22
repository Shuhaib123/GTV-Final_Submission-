package instrumenter

import (
	"bytes"

	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"strings"
)

// (Options and helpers moved to options.go)

// InstrumentProgram converts a Go 'main' program into a workload file that
// exports `Run<Name>Program(ctx context.Context)` and registers it in the
// workload registry. It performs light-weight region insertion based on
// gtv:send / gtv:recv comment markers immediately preceding statements.
//
// Example markers:
//
//	// gtv:send=left[2]
//	ch <- v
//	// gtv:recv=left[2]
//	v := <-ch
//	// gtv:role=client (sets role log at function entry)
func InstrumentProgram(src []byte, name string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "input.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Change package to workload.
	file.Name.Name = "workload"

	// Ensure required imports
	EnsureImports(file, "context", "runtime/trace")
	commentMap := ast.NewCommentMap(fset, file, file.Comments)
	leadingCommentGroup := func(n ast.Node) *ast.CommentGroup {
		if n == nil {
			return nil
		}
		groups := commentMap[n]
		var last *ast.CommentGroup
		for _, cg := range groups {
			if cg == nil {
				continue
			}
			if cg.End() > n.Pos() {
				continue
			}
			if last == nil || cg.Pos() > last.Pos() {
				last = cg
			}
		}
		if last != nil {
			return last
		}
		for _, cg := range file.Comments {
			if cg.End() > n.Pos() {
				break
			}
			if cg.Pos() < n.Pos() && cg.End() <= n.Pos() {
				last = cg
			}
		}
		return last
	}
	// provide a small wrapper to keep existing call sites that use a local ensureImport
	ensureImport := func(path string) { EnsureImport(file, path) }

	// Merge options/env/config/directives (Pass 0 extraction)
	opts, onlySet, skipSet, timeoutNS, hasTimeout, doneName := ParseOptionsFromEnvAndDirectives(file, instrOpts)
	_ = onlySet
	_ = skipSet
	_ = timeoutNS
	_ = hasTimeout
	_ = doneName

	// Find main() and convert to Run<Name>Program(ctx context.Context)
	runName := fmt.Sprintf("Run%sProgram", name)
	var runFn *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "main" {
			return true
		}
		fn.Name.Name = runName
		// params: (ctx context.Context)
		fn.Type.Params = &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent("ctx")},
			Type:  &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Context")},
		}}}
		// add task scope at top using AST nodes (avoid string-parsed stmts)
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("task")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("NewTask")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", name)}}}},
		}
		deferTask := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("task"), Sel: ast.NewIdent("End")}}}
		logStart := &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: "\"main\""}, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s starting\"", name)}}}}
		fn.Body.List = append([]ast.Stmt{assign, deferTask, logStart}, fn.Body.List...)
		runFn = fn
		return false
	})

	// Propagate ctx to other functions and optionally log roles declared via gtv:role= in doc.
	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		if fn.Name.Name == runName {
			continue
		}
		// Determine if ctx param already exists as first param
		hasCtx := false
		if fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
				if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
					hasCtx = true
				}
			}
		}
		// Skip adding ctx to HTTP handlers (func(w http.ResponseWriter, r *http.Request) or methods with same params)
		isHTTP := false
		if fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) >= 2 {
			if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
				if xid, ok := se.X.(*ast.Ident); ok && xid.Name == "http" && se.Sel.Name == "ResponseWriter" {
					if star, ok := fn.Type.Params.List[1].Type.(*ast.StarExpr); ok {
						if se2, ok := star.X.(*ast.SelectorExpr); ok {
							if xid2, ok := se2.X.(*ast.Ident); ok && xid2.Name == "http" && se2.Sel.Name == "Request" {
								isHTTP = true
							}
						}
					}
				}
			}
		}
		if !hasCtx && !isHTTP {
			// Insert ctx as first parameter
			ensureImport("context")
			fld := &ast.Field{Names: []*ast.Ident{ast.NewIdent("ctx")}, Type: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Context")}}
			if fn.Type.Params == nil {
				fn.Type.Params = &ast.FieldList{}
			}
			fn.Type.Params.List = append([]*ast.Field{fld}, fn.Type.Params.List...)
		}
		// No need to record; call-site pass below inspects file declarations directly.
		// Role logging via function doc comments: // gtv:role=server
		// HTTP task via function doc: // gtv:http=GET /path or any string name
		// gRPC task via function doc: // gtv:grpc=grpc.MethodName
		// Generic task via function doc: // gtv:task=TaskName
		var role string
		var httpName string
		var grpcName string
		var genericTask string
		type idSpec struct{ Expr, Key string }
		var ids []idSpec
		if fn.Doc != nil {
			for _, c := range fn.Doc.List {
				txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
				if strings.HasPrefix(txt, "gtv:role=") {
					role = strings.TrimSpace(strings.TrimPrefix(txt, "gtv:role="))
					role = strings.Trim(role, "`\"")
				} else if strings.HasPrefix(txt, "gtv:http=") {
					httpName = strings.TrimSpace(strings.TrimPrefix(txt, "gtv:http="))
					httpName = strings.Trim(httpName, "`\"")
				} else if strings.HasPrefix(txt, "gtv:grpc=") {
					grpcName = strings.TrimSpace(strings.TrimPrefix(txt, "gtv:grpc="))
					grpcName = strings.Trim(grpcName, "`\"")
				} else if strings.HasPrefix(txt, "gtv:task=") {
					genericTask = strings.TrimSpace(strings.TrimPrefix(txt, "gtv:task="))
					genericTask = strings.Trim(genericTask, "`\"")
				} else if strings.HasPrefix(txt, "gtv:id=") {
					raw := strings.TrimSpace(strings.TrimPrefix(txt, "gtv:id="))
					parts := strings.Split(raw, ",")
					for _, p := range parts {
						p = strings.TrimSpace(strings.Trim(p, "`\""))
						if p == "" {
							continue
						}
						expr := p
						key := p
						if i := strings.Index(p, ":"); i >= 0 {
							expr = strings.TrimSpace(p[:i])
							key = strings.TrimSpace(p[i+1:])
						} else {
							if j := strings.LastIndex(p, "."); j >= 0 && j+1 < len(p) {
								key = p[j+1:]
							}
						}
						ids = append(ids, idSpec{Expr: expr, Key: key})
					}
				}
			}
		}
		// Auto-detect http handlers when enabled and no explicit httpName set.
		if httpName == "" && opts.AddHTTPHandlerTasks && fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) >= 2 {
			// First param: http.ResponseWriter, second: *http.Request
			p0 := fn.Type.Params.List[0]
			p1 := fn.Type.Params.List[1]
			if se, ok := p0.Type.(*ast.SelectorExpr); ok {
				if xid, ok := se.X.(*ast.Ident); ok && xid.Name == "http" && se.Sel.Name == "ResponseWriter" {
					if star, ok := p1.Type.(*ast.StarExpr); ok {
						if se2, ok := star.X.(*ast.SelectorExpr); ok {
							if xid2, ok := se2.X.(*ast.Ident); ok && xid2.Name == "http" && se2.Sel.Name == "Request" {
								httpName = "http.handler"
							}
						}
					}
				}
			}
		}
		if role != "" && fn.Body != nil && (opts.Level == "regions_logs" || opts.Level == "") {
			ensureImport("fmt")
			// trace.Log(ctx, "<role>", fmt.Sprintf("%s running", role))
			logCall := &ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")},
				Args: []ast.Expr{
					ast.NewIdent("ctx"),
					&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", role)},
					&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Sprintf")}, Args: []ast.Expr{
						&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", role+" running")},
					}},
				},
			}}
			// wrap in if trace.IsEnabled()
			ifStmt := &ast.IfStmt{Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("IsEnabled")}}, Body: &ast.BlockStmt{List: []ast.Stmt{logCall}}}
			fn.Body.List = append([]ast.Stmt{ifStmt}, fn.Body.List...)
		}
		if genericTask != "" && fn.Body != nil {
			ensureImport("context")
			ensureImport("runtime/trace")
			baseCtx := ast.Expr(ast.NewIdent("ctx"))
			// if no ctx param, use context.Background()
			hasParamCtx := false
			if fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
				if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
						hasParamCtx = true
					}
				}
			}
			if !hasParamCtx {
				baseCtx = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Background")}}
			}
			assign := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("task")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("NewTask")}, Args: []ast.Expr{baseCtx, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", genericTask)}}}}}
			deferTask := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("task"), Sel: ast.NewIdent("End")}}}
			// Optional ID logs at task start
			if len(ids) == 0 {
				if fn.Type != nil && fn.Type.Params != nil {
					count := 0
					for _, fld := range fn.Type.Params.List {
						for _, nm := range fld.Names {
							n := nm.Name
							ln := strings.ToLower(n)
							if strings.HasSuffix(ln, "id") {
								ids = append(ids, idSpec{Expr: n, Key: n})
								count++
								if count >= 3 {
									break
								}
							}
						}
						if len(ids) >= 3 {
							break
						}
					}
				}
			}
			stmts := []ast.Stmt{assign}
			if len(ids) > 0 {
				ensureImport("fmt")
				for _, sp := range ids {
					stmts = append(stmts, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "id.%s", fmt.Sprint(%s)) }`, sp.Key, sp.Expr)))
				}
			}
			stmts = append(stmts, deferTask)
			fn.Body.List = append(stmts, fn.Body.List...)
		}
		// HTTP task root: insert a task at function entry if httpName is set
		if httpName != "" && fn.Body != nil {
			ensureImport("context")
			ensureImport("runtime/trace")
			// base context: prefer existing ctx, else r.Context() if available, else context.Background()
			baseCtx := ast.Expr(ast.NewIdent("ctx"))
			if !(hasCtx) {
				// try r.Context() from param #2
				if fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) >= 2 {
					p1 := fn.Type.Params.List[1]
					if len(p1.Names) > 0 {
						rname := p1.Names[0].Name
						baseCtx = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(rname), Sel: ast.NewIdent("Context")}}
					} else {
						baseCtx = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Background")}}
					}
				}
			}
			// ctx, task := trace.NewTask(baseCtx, httpName); defer task.End()
			assign := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("task")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("NewTask")}, Args: []ast.Expr{baseCtx, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", httpName)}}}}}
			deferTask := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("task"), Sel: ast.NewIdent("End")}}}
			// Optional ID logs at task start (skip first two params w,r)
			if len(ids) == 0 {
				if fn.Type != nil && fn.Type.Params != nil {
					count := 0
					for i, fld := range fn.Type.Params.List {
						if i < 2 {
							continue
						}
						for _, nm := range fld.Names {
							n := nm.Name
							ln := strings.ToLower(n)
							if strings.HasSuffix(ln, "id") {
								ids = append(ids, idSpec{Expr: n, Key: n})
								count++
								if count >= 3 {
									break
								}
							}
						}
						if len(ids) >= 3 {
							break
						}
					}
				}
			}
			stmts := []ast.Stmt{assign}
			if len(ids) > 0 {
				ensureImport("fmt")
				for _, sp := range ids {
					stmts = append(stmts, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "id.%s", fmt.Sprint(%s)) }`, sp.Key, sp.Expr)))
				}
			}
			stmts = append(stmts, deferTask)
			fn.Body.List = append(stmts, fn.Body.List...)
		}
		// gRPC task root: insert a task when grpcName is set or auto-detected
		if grpcName == "" && opts.AddGRPCTasks && fn.Type != nil && fn.Type.Params != nil {
			// Unary: method with first parameter context.Context
			if fn.Recv != nil && len(fn.Type.Params.List) >= 1 {
				if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
						grpcName = "grpc." + fn.Name.Name
					}
				}
			}
			// Stream: method with single param *SomethingServer or SomethingServer
			if grpcName == "" && fn.Recv != nil && len(fn.Type.Params.List) == 1 {
				switch t := fn.Type.Params.List[0].Type.(type) {
				case *ast.StarExpr:
					if se, ok := t.X.(*ast.SelectorExpr); ok && strings.HasSuffix(se.Sel.Name, "Server") {
						grpcName = "grpc.stream." + fn.Name.Name
					}
				case *ast.SelectorExpr:
					if strings.HasSuffix(t.Sel.Name, "Server") {
						grpcName = "grpc.stream." + fn.Name.Name
					}
				}
			}
		}
		if grpcName != "" && fn.Body != nil {
			ensureImport("context")
			ensureImport("runtime/trace")
			// Determine base ctx
			baseCtx := ast.Expr(ast.NewIdent("ctx"))
			hasCtxParam := false
			if fn.Type != nil && fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
				if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
						hasCtxParam = true
					}
				}
			}
			if !hasCtxParam {
				if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
					p := fn.Type.Params.List[0]
					if len(p.Names) > 0 {
						baseCtx = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(p.Names[0].Name), Sel: ast.NewIdent("Context")}}
					} else {
						baseCtx = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Background")}}
					}
				}
			}
			assign := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("task")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("NewTask")}, Args: []ast.Expr{baseCtx, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", grpcName)}}}}}
			deferTask := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("task"), Sel: ast.NewIdent("End")}}}
			if len(ids) == 0 {
				if fn.Type != nil && fn.Type.Params != nil {
					count := 0
					for _, fld := range fn.Type.Params.List {
						for _, nm := range fld.Names {
							n := nm.Name
							ln := strings.ToLower(n)
							if strings.HasSuffix(ln, "id") {
								ids = append(ids, idSpec{Expr: n, Key: n})
								count++
								if count >= 3 {
									break
								}
							}
						}
						if len(ids) >= 3 {
							break
						}
					}
				}
			}
			stmts := []ast.Stmt{assign}
			if len(ids) > 0 {
				ensureImport("fmt")
				for _, sp := range ids {
					stmts = append(stmts, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "id.%s", fmt.Sprint(%s)) }`, sp.Key, sp.Expr)))
				}
			}
			stmts = append(stmts, deferTask)
			fn.Body.List = append(stmts, fn.Body.List...)
		}
	}

	// Update call sites to pass ctx for functions/methods we modified.
	// Be conservative: only inject for local functions/methods defined in this file
	// and skip selectors that clearly refer to imported package aliases.
	// Build a set of import aliases/package names for quick checks.
	importAliases := make(map[string]bool)
	importPaths := make(map[string]string) // alias -> full import path
	for _, imp := range file.Imports {
		var alias string
		if imp.Name != nil {
			alias = imp.Name.Name
			importAliases[alias] = true
		} else {
			p := strings.Trim(imp.Path.Value, "\"")
			// derive last path segment
			i := strings.LastIndex(p, "/")
			base := p
			if i >= 0 && i+1 < len(p) {
				base = p[i+1:]
			}
			alias = base
			importAliases[base] = true
		}
		importPaths[alias] = strings.Trim(imp.Path.Value, "\"")
	}
	hasDBImport := false
	for _, p := range importPaths {
		if p == "database/sql" {
			hasDBImport = true
			break
		}
	}
	// JSON helpers
	jsonRegionLabel := func(name string) string {
		switch name {
		case "Unmarshal":
			return "json.unmarshal"
		case "Marshal", "MarshalIndent":
			return "json.marshal"
		default:
			return ""
		}
	}
	isJSONCall := func(call *ast.CallExpr) (*ast.SelectorExpr, bool) {
		if call == nil {
			return nil, false
		}
		se, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || se.Sel == nil {
			return nil, false
		}
		if xid, ok := se.X.(*ast.Ident); ok {
			if importPaths[xid.Name] == "encoding/json" {
				switch se.Sel.Name {
				case "Marshal", "MarshalIndent", "Unmarshal":
					return se, true
				}
			}
		}
		return nil, false
	}
	// Effective IO feature flags per library
	enableJSON := opts.AddIORegions || opts.AddIOJSONRegions
	enableDB := opts.AddIORegions || opts.AddIODBRegions
	enableHTTP := opts.AddIORegions || opts.AddIOHTTPRegions
	enableOS := opts.AddIORegions || opts.AddIOOSRegions
	hasFirstParamCtx := func(fd *ast.FuncDecl) bool {
		if fd == nil || fd.Type == nil || fd.Type.Params == nil || len(fd.Type.Params.List) == 0 {
			return false
		}
		if se, ok := fd.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
			if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
				return true
			}
		}
		return false
	}
	ast.Inspect(file, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := ce.Fun.(type) {
		case *ast.Ident:
			// function call: search local declarations for a matching function with ctx
			for _, d := range file.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name != nil && fd.Name.Name == fun.Name {
					if hasFirstParamCtx(fd) {
						if !(len(ce.Args) > 0 && isCtxIdent(ce.Args[0])) {
							ce.Args = append([]ast.Expr{ast.NewIdent("ctx")}, ce.Args...)
						}
					}
					break
				}
			}
		case *ast.SelectorExpr:
			// method or package selector; if X is an import alias, skip
			if id, ok := fun.X.(*ast.Ident); ok {
				if importAliases[id.Name] {
					return true
				}
			}
			// otherwise, treat as method on local type; match by method name
			for _, d := range file.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Name != nil && fd.Name.Name == fun.Sel.Name {
					if hasFirstParamCtx(fd) {
						if !(len(ce.Args) > 0 && isCtxIdent(ce.Args[0])) {
							ce.Args = append([]ast.Expr{ast.NewIdent("ctx")}, ce.Args...)
						}
					}
					break
				}
			}
		}
		return true
	})

	// Do not alter other function signatures or call sites.
	// Rationale: safer transform that mirrors manual wrapping style; only main is transformed
	// into Run<Name>Program(ctx). All channel send/recv instrumentation relies on gtv markers.

	// Utility: trim both quoted and backtick-wrapped values.
	trimQ := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.Trim(s, "\"")
		s = strings.Trim(s, "`")
		return s
	}

	// Light instrumentation via comments: wrap marked statements.
	// Supports either simple lines (gtv:send=..., gtv:recv=..., gtv:role=...)
	// or comma-separated key=val pairs after gtv:, e.g. gtv:label=clientout[i],role=client,value=msg
	type ann struct{ send, recv, label, role, value, phase, phaseStart, phaseEnd, funcName, loop string }
	parseAnn := func(cmts string) ann {
		var a ann
		for _, line := range strings.Split(cmts, "\n") {
			p := strings.TrimSpace(strings.TrimPrefix(line, "//"))
			if !strings.HasPrefix(p, "gtv:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(p, "gtv:"))
			// allow simple forms
			if strings.HasPrefix(payload, "send=") {
				a.send = strings.TrimSpace(strings.TrimPrefix(payload, "send="))
				continue
			}
			if strings.HasPrefix(payload, "recv=") {
				a.recv = strings.TrimSpace(strings.TrimPrefix(payload, "recv="))
				continue
			}
			if strings.HasPrefix(payload, "role=") {
				a.role = strings.TrimSpace(strings.TrimPrefix(payload, "role="))
				continue
			}
			// key=val pairs: label=...,role=...,value=...
			for _, part := range strings.Split(payload, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				kv := strings.SplitN(part, "=", 2)
				key := strings.TrimSpace(kv[0])
				val := ""
				if len(kv) > 1 {
					val = strings.TrimSpace(kv[1])
				}
				switch key {
				case "send":
					a.send = val
				case "recv":
					a.recv = val
				case "label":
					a.label = val
				case "role":
					a.role = val
				case "value":
					a.value = val
				case "phase":
					a.phase = trimQ(val) // allow quoted or plain
				case "phase_start":
					a.phaseStart = trimQ(val)
				case "phase_end":
					a.phaseEnd = trimQ(val)
				case "func":
					a.funcName = trimQ(val)
				case "loop":
					a.loop = trimQ(val)
				}
			}
		}
		return a
	}

	// direction inference helpers were previously here; removed since auto-wrapping
	// logic below detects send/recv by statement kind directly.

	// Recursive, statement-aware instrumentation that also auto-wraps unannotated
	// channel operations (send/recv) with a fallback label derived from the
	// channel expression.

	// helper builds a user-facing label from a channel expression
	// examples:
	//   ch           -> "ch"
	//   s.clientOut  -> "clientout"
	//   clientOut[i] -> "clientout[i]"
	//   s.clientOut[i] -> "clientout[i]"
	labelFromChan := func(ch ast.Expr) (ast.Expr, bool) {
		// base name
		base := "chan"
		var idx ast.Expr
		// peel index if any
		switch ce := ch.(type) {
		case *ast.IndexExpr:
			idx = ce.Index
			ch = ce.X
		}
		switch ce := ch.(type) {
		case *ast.Ident:
			base = ce.Name
		case *ast.SelectorExpr:
			base = ce.Sel.Name
		}
		base = strings.ToLower(base)
		if idx == nil {
			return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", base)}, false
		}
		// support identifier or literal index directly; otherwise, just use base
		switch ix := idx.(type) {
		case *ast.Ident:
			// dynamic label with ident index: use buildLabelExpr("base[ix]") which auto fmt-imports
			lbl, need := buildLabelExpr(fmt.Sprintf("%s[%s]", base, ix.Name))
			return lbl, need
		case *ast.BasicLit:
			return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s[%s]\"", base, ix.Value)}, false
		default:
			return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", base)}, false
		}
	}

	buildSendRecvLabel := func(kind string, ch ast.Expr, meta ann, fnRole string) (ast.Expr, bool) {
		lbl, needFmt := labelFromChan(ch)
		prefix := fnRole
		if prefix == "" {
			prefix = "worker"
		}
		switch kind {
		case "recv":
			lbl = addPrefix(lbl, fmt.Sprintf("%s: receive from ", prefix))
		case "send":
			lbl = addPrefix(lbl, fmt.Sprintf("%s: send to ", prefix))
		}
		return lbl, needFmt
	}

	channelExprFromClause := func(c *ast.CommClause) ast.Expr {
		if c == nil || c.Comm == nil {
			return nil
		}
		switch stmt := c.Comm.(type) {
		case *ast.AssignStmt:
			if len(stmt.Rhs) == 1 {
				if ue, ok := stmt.Rhs[0].(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
					return ue.X
				}
			}
		case *ast.ExprStmt:
			if ue, ok := stmt.X.(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
				return ue.X
			}
		}
		return nil
	}

	isReceiveClause := func(c *ast.CommClause) bool {
		return channelExprFromClause(c) != nil
	}

	buildReceiveLabel := func(prefix string, c *ast.CommClause) (ast.Expr, bool) {
		ch := channelExprFromClause(c)
		if ch == nil {
			return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s receive\"", prefix)}, false
		}
		lbl, needFmt := labelFromChan(ch)
		return addPrefix(lbl, fmt.Sprintf("%s: receive from ", prefix)), needFmt
	}

	// pretty-prints an AST expression back to source text (best-effort)
	exprString := func(e ast.Expr) string {
		if e == nil {
			return ""
		}
		var b bytes.Buffer
		_ = printer.Fprint(&b, fset, e)
		return b.String()
	}

	detectSelectComm := func(comm ast.Stmt, meta ann) (string, ast.Expr, string) {
		if comm == nil {
			return "", nil, ""
		}
		switch stmt := comm.(type) {
		case *ast.SendStmt:
			return "send", stmt.Chan, meta.value
		case *ast.AssignStmt:
			if len(stmt.Rhs) == 1 {
				if ue, ok := stmt.Rhs[0].(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
					val := meta.value
					if val == "" && len(stmt.Lhs) > 0 {
						val = exprString(stmt.Lhs[0])
					}
					return "recv", ue.X, val
				}
			}
		case *ast.ExprStmt:
			if ue, ok := stmt.X.(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
				return "recv", ue.X, meta.value
			}
		}
		return "", nil, ""
	}

	// injectCopiesInGoLiterals prepends shadowing copies of the given identifier names
	// at the top of any func literal used as a goroutine within the provided block.
	// Example: for each name "i" it inserts `i := i` as the first statement.
	injectCopiesInGoLiterals := func(block *ast.BlockStmt, names []string) {
		if block == nil || len(names) == 0 {
			return
		}
		// de-dup names and ignore blanks
		uniq := make([]string, 0, len(names))
		seen := make(map[string]bool)
		for _, n := range names {
			if n == "" || n == "_" {
				continue
			}
			if !seen[n] {
				seen[n] = true
				uniq = append(uniq, n)
			}
		}
		if len(uniq) == 0 {
			return
		}
		// local traversal for copying loop vars in go-literals does not need select counters
		var walkBlock func(*ast.BlockStmt)
		walkBlock = func(b *ast.BlockStmt) {
			for _, st := range b.List {
				switch s := st.(type) {
				case *ast.GoStmt:
					if ce := s.Call; ce != nil {
						if lit, ok := ce.Fun.(*ast.FuncLit); ok && lit.Body != nil {
							// Remove names that are already self-copied at the top of the body
							filter := func(list []string) []string {
								out := make([]string, 0, len(list))
								for _, nm := range list {
									has := false
									// Scan first few statements for `nm := nm`
									scanN := 3
									for i := 0; i < len(lit.Body.List) && i < scanN; i++ {
										if as, ok := lit.Body.List[i].(*ast.AssignStmt); ok && as.Tok == token.DEFINE {
											if len(as.Lhs) == 1 && len(as.Rhs) == 1 {
												if lid, ok := as.Lhs[0].(*ast.Ident); ok && lid.Name == nm {
													if rid, ok := as.Rhs[0].(*ast.Ident); ok && rid.Name == nm {
														has = true
														break
													}
												}
											}
										}
									}
									if !has {
										out = append(out, nm)
									}
								}
								return out
							}
							names2 := filter(uniq)
							if len(names2) == 0 {
								continue
							}
							// Build copies and prepend to body
							copies := make([]ast.Stmt, 0, len(names2))
							for _, nm := range names2 {
								copies = append(copies, &ast.AssignStmt{
									Lhs: []ast.Expr{ast.NewIdent(nm)},
									Tok: token.DEFINE,
									Rhs: []ast.Expr{ast.NewIdent(nm)},
								})
							}
							lit.Body.List = append(copies, lit.Body.List...)
						}
					}
				case *ast.BlockStmt:
					walkBlock(s)
				case *ast.IfStmt:
					if s.Body != nil {
						walkBlock(s.Body)
					}
					if b, ok := s.Else.(*ast.BlockStmt); ok {
						walkBlock(b)
					}
				case *ast.ForStmt:
					if s.Body != nil {
						walkBlock(s.Body)
					}
				case *ast.RangeStmt:
					if s.Body != nil {
						walkBlock(s.Body)
					}
				case *ast.SwitchStmt:
					if s.Body != nil {
						for _, cc := range s.Body.List {
							if c, ok := cc.(*ast.CaseClause); ok {
								tmp := &ast.BlockStmt{List: c.Body}
								walkBlock(tmp)
								c.Body = tmp.List
							}
						}
					}
				case *ast.SelectStmt:
					// Traverse into select case bodies to handle go-literal copies
					if s.Body != nil {
						for _, cc := range s.Body.List {
							if c, ok := cc.(*ast.CommClause); ok {
								tmp := &ast.BlockStmt{List: c.Body}
								walkBlock(tmp)
								c.Body = tmp.List
							}
						}
					}
				}
			}
		}
		walkBlock(block)
	}

	// Guarded region wrapper: if gated is true, generate
	//   if trace.IsEnabled() { trace.WithRegion(ctx, label, func(){ stmt }) } else { stmt }
	// to avoid building dynamic labels when tracing is disabled.
	wrapWithRegionExprGuarded := func(label ast.Expr, stmt ast.Stmt, gated bool) ast.Stmt {
		return WrapWithRegionExpr(label, stmt, gated)
	}

	// counter for StartRegion/End wrappers
	regCounter := 0
	// wrapWithStartEndCtxExpr wraps a statement with StartRegion/End to preserve := scope
	wrapWithStartEndCtxExpr := func(label ast.Expr, ctxExpr ast.Expr, stmt ast.Stmt) ast.Stmt {
		regCounter++
		varName := fmt.Sprintf("__gtvReg%d", regCounter)
		// __gtvRegN := trace.StartRegion(ctxExpr, label)
		start := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(varName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("StartRegion")}, Args: []ast.Expr{ctxExpr, label}}}}
		// __gtvRegN.End()
		end := &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(varName), Sel: ast.NewIdent("End")}}}
		return &ast.BlockStmt{List: []ast.Stmt{start, stmt, end}}
	}

	usedWG := false
	// collect soft warnings to stderr
	var warnings []string
	posString := func(n ast.Node) string {
		p := fset.Position(n.Pos())
		return fmt.Sprintf("%s:%d", p.Filename, p.Line)
	}
	// Optional: type info to improve DB detection
	var typeInfo types.Info
	var typeChecked bool
	{
		// Build a minimal types.Info so we can recognize receivers for database/sql
		conf := &types.Config{Importer: importer.Default(), Error: func(err error) { /* ignore; best-effort */ }}
		typeInfo = types.Info{
			Types: make(map[ast.Expr]types.TypeAndValue),
			Uses:  make(map[*ast.Ident]types.Object),
		}
		// types.Check works on a package; treat this single file as a package
		if _, err := conf.Check("instrument", fset, []*ast.File{file}, &typeInfo); err == nil {
			typeChecked = true
		}
	}
	// Helper: detect whether a selector is a method on *sql.DB, *sql.Tx, or *sql.Stmt
	isSQLSelector := func(sel *ast.SelectorExpr) (recvName string, method string, ok bool) {
		if !typeChecked || sel == nil || sel.Sel == nil {
			return "", "", false
		}
		t := typeInfo.Types[sel.X].Type
		if t == nil {
			return "", "", false
		}
		// Deref pointers
		if p, okp := t.(*types.Pointer); okp {
			t = p.Elem()
		}
		n, okn := t.(*types.Named)
		if !okn {
			return "", "", false
		}
		obj := n.Obj()
		if obj == nil || obj.Pkg() == nil {
			return "", "", false
		}
		if obj.Pkg().Path() != "database/sql" {
			return "", "", false
		}
		tn := obj.Name()
		if tn != "DB" && tn != "Tx" && tn != "Stmt" {
			return "", "", false
		}
		return tn, sel.Sel.Name, true
	}
	var instrumentStmts func([]ast.Stmt, bool, string) []ast.Stmt
	phaseCounter := 0
	phaseStack := make([]string, 0)
	// track generated function names to avoid duplicates
	generated := make(map[string]bool)
	// whether a bare `return` is valid in the enclosing function
	allowBareReturn := false
	selectRecvCounter := 0
	spawnCounter := 0
	usedSpawn := false
	instrumentStmts = func(list []ast.Stmt, hasCtx bool, fnRole string) []ast.Stmt {
		rewriteSelectClauseAssign := func(c *ast.CommClause) {
			if c == nil {
				return
			}
			assign, ok := c.Comm.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 {
				return
			}
			if ue, ok := assign.Rhs[0].(*ast.UnaryExpr); !ok || ue.Op != token.ARROW {
				return
			} else {
				selectRecvCounter++
				tmpName := fmt.Sprintf("__gtvRecv%d", selectRecvCounter)
				tmp := ast.NewIdent(tmpName)
				c.Comm = &ast.AssignStmt{
					Lhs: []ast.Expr{tmp},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{ue},
				}
				assignCopy := &ast.AssignStmt{
					Lhs: assign.Lhs,
					Tok: assign.Tok,
					Rhs: []ast.Expr{ast.NewIdent(tmpName)},
				}
				c.Body = append([]ast.Stmt{assignCopy}, c.Body...)
			}
		}
		wrapSelectReceive := func(c *ast.CommClause) {
			if !hasCtx || !isReceiveClause(c) || len(c.Body) == 0 {
				return
			}
			block := &ast.BlockStmt{List: c.Body}
			if !loopBodySafe(block) {
				warnings = append(warnings, fmt.Sprintf("gtv:recv ignored at %s (select clause not safe)", posString(c)))
				return
			}
			prefix := "worker"
			if fnRole != "" {
				prefix = fnRole
			}
			label, needFmt := buildReceiveLabel(prefix, c)
			if needFmt {
				ensureImport("fmt")
			}
			gated := opts.GuardDynamicLabels && needFmt
			chExpr := channelExprFromClause(c)
			stmts := make([]ast.Stmt, 0, len(c.Body)+1)
			if chExpr != nil {
				stmts = append(stmts, EmitChPtrLog(chExpr))
			}
			stmts = append(stmts, c.Body...)
			c.Body = []ast.Stmt{wrapWithRegionExprGuarded(label, &ast.BlockStmt{List: stmts}, gated)}
		}
		outs := make([]ast.Stmt, 0, len(list))
		for _, st := range list {
			// read leading comments for annotations
			var cmts string
			if cg := leadingCommentGroup(st); cg != nil {
				cmts = commentText(cg)
			}
			meta := parseAnn(cmts)
			// phase regions/logs just before instrumenting the statement
			if meta.phaseStart != "" && hasCtx {
				phaseCounter++
				varName := fmt.Sprintf("__gtvPhase%d", phaseCounter)
				// __gtvPhaseN := trace.StartRegion(ctx, "phase: <text>")
				start := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(varName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("StartRegion")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", "phase: "+meta.phaseStart)}}}}}
				outs = append(outs, start)
				// optional log
				outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: "\"main\""}, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", meta.phaseStart)}}}})
				phaseStack = append(phaseStack, varName)
			}
			// simple phase/role logs only if ctx in scope and level allows logs
			if hasCtx && (opts.Level == "regions_logs" || opts.Level == "") {
				if meta.phase != "" {
					outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: "\"main\""}, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", meta.phase)}}}})
				}
				if meta.role != "" {
					outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: "\"role\""}, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", meta.role)}}}})
				}
			}

			// instrumentation dispatch based on statement kind
			switch s := st.(type) {
			case *ast.BlockStmt:
				// Function extraction: if gtv:func=Name is set, lift this block via a function wrapper
				if meta.funcName != "" {
					// instrument the body before extraction
					fnBody := instrumentStmts(s.List, hasCtx, fnRole)
					// define the function once: func Name(ctx context.Context, fn func()) { fn() }
					if !generated[meta.funcName] {
						newFn := &ast.FuncDecl{
							Name: ast.NewIdent(meta.funcName),
							Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{
								{
									Names: []*ast.Ident{ast.NewIdent("ctx")},
									Type:  &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Context")},
								},
								{
									Names: []*ast.Ident{ast.NewIdent("fn")},
									Type:  &ast.FuncType{Params: &ast.FieldList{}},
								},
							}}},
							Body: &ast.BlockStmt{List: []ast.Stmt{mkExpr("fn()")}},
						}
						file.Decls = append(file.Decls, newFn)
						generated[meta.funcName] = true
					}
					// replace block with call: Name(ctx, func(){ <fnBody> })
					lit := &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: fnBody}}
					call := &ast.CallExpr{Fun: ast.NewIdent(meta.funcName), Args: []ast.Expr{ast.NewIdent("ctx"), lit}}
					outs = append(outs, &ast.ExprStmt{X: call})
					// phase end handling still applies after call
					if meta.phaseEnd != "" && hasCtx {
						if n := len(phaseStack); n > 0 {
							varName := phaseStack[n-1]
							phaseStack = phaseStack[:n-1]
							outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(varName), Sel: ast.NewIdent("End")}}})
							outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: "\"main\""}, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", meta.phaseEnd)}}}})
						}
					}
					continue
				}
				// otherwise, recurse into block
				s.List = instrumentStmts(s.List, hasCtx, fnRole)
				outs = append(outs, st)
			case *ast.GoStmt:
				// Ensure goroutine contributes to WaitGroup
				usedWG = true
				sidName := ""
				if hasCtx {
					spawnCounter++
					sidName = fmt.Sprintf("__gtvSpawn%d", spawnCounter)
					ensureImport("sync/atomic")
					ensureImport("fmt")
					assign := &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(sidName)},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{&ast.CallExpr{
							Fun: &ast.SelectorExpr{X: ast.NewIdent("atomic"), Sel: ast.NewIdent("AddUint64")},
							Args: []ast.Expr{
								&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent("gtvSpawnID")},
								&ast.BasicLit{Kind: token.INT, Value: "1"},
							},
						}},
					}
					outs = append(outs, assign)
					parentLog := mkExpr(fmt.Sprintf(`trace.Log(ctx, "spawn_parent", fmt.Sprintf("sid=%%d", %s))`, sidName))
					outs = append(outs, parentLog)
					usedSpawn = true
				}
				// Insert wg.Add(1) before the go statement
				outs = append(outs, mkExpr("wg.Add(1)"))
				// Modify the goroutine to call Done() at exit without mutating original node
				if ce := s.Call; ce != nil {
					switch fun := ce.Fun.(type) {
					case *ast.FuncLit:
						// clone the func literal header and body list (shallow) to avoid side effects
						bodyStmts := []ast.Stmt{&ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("wg"), Sel: ast.NewIdent("Done")}}}}
						if hasCtx && sidName != "" {
							childLog := mkExpr(fmt.Sprintf(`trace.Log(ctx, "spawn_child", fmt.Sprintf("sid=%%d", %s))`, sidName))
							bodyStmts = append(bodyStmts, childLog)
						}
						bodyStmts = append(bodyStmts, fun.Body.List...)
						newFun := &ast.FuncLit{Type: fun.Type, Body: &ast.BlockStmt{List: bodyStmts}}
						newGo := &ast.GoStmt{Call: &ast.CallExpr{Fun: newFun, Args: ce.Args}}
						outs = append(outs, newGo)
					default:
						// wrap any other call into a func literal to add defer wg.Done()
						callCopy := *ce
						lit := &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &callCopy}}}}
						bodyStmts := []ast.Stmt{&ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("wg"), Sel: ast.NewIdent("Done")}}}}
						if hasCtx && sidName != "" {
							childLog := mkExpr(fmt.Sprintf(`trace.Log(ctx, "spawn_child", fmt.Sprintf("sid=%%d", %s))`, sidName))
							bodyStmts = append(bodyStmts, childLog)
						}
						bodyStmts = append(bodyStmts, lit.Body.List...)
						lit.Body.List = bodyStmts
						newGo := &ast.GoStmt{Call: &ast.CallExpr{Fun: lit}}
						outs = append(outs, newGo)
					}
				} else {
					// not a call expr; leave as-is
					outs = append(outs, st)
				}
			case *ast.SendStmt:
				// choose label
				var lbl ast.Expr
				var needFmt bool
				prefix := "worker"
				if fnRole != "" {
					prefix = fnRole
				}
				if meta.send != "" || (meta.label != "" && meta.recv == "") {
					tag := meta.send
					if tag == "" {
						tag = meta.label
					}
					lbl, needFmt = buildLabelExpr(fmt.Sprintf("%s: send to %s", prefix, tag))
				} else {
					l, nf := labelFromChan(s.Chan)
					lbl = addPrefix(l, prefix+": send to ")
					needFmt = nf
				}
				if needFmt {
					ensureImport("fmt")
				}
				if hasCtx {
					gated := opts.GuardDynamicLabels && needFmt
					// For sends, log the value BEFORE the region so it attaches to send_attempt
					valExpr := meta.value
					if valExpr == "" {
						valExpr = exprString(s.Value)
					}
					if valExpr != "" && (opts.Level == "regions_logs" || opts.Level == "") {
						ensureImport("fmt")
						outs = append(outs, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "value", fmt.Sprint(%s)) }`, valExpr)))
					}
					// Log channel pointer identity before entering the region
					ensureImport("fmt")
					ptrLog := mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%%p", %s)) }`, exprString(s.Chan)))
					blk := &ast.BlockStmt{List: []ast.Stmt{st}}
					outs = append(outs, ptrLog)
					outs = append(outs, wrapWithRegionExprGuarded(lbl, blk, gated))
				} else {
					outs = append(outs, st)
				}

			case *ast.ExprStmt:
				// possible bare receive: <-ch
				if ue, ok := s.X.(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
					var lbl ast.Expr
					var needFmt bool
					prefix := "worker"
					if fnRole != "" {
						prefix = fnRole
					}
					if meta.recv != "" || (meta.label != "" && meta.send == "") {
						tag := meta.recv
						if tag == "" {
							tag = meta.label
						}
						lbl, needFmt = buildLabelExpr(fmt.Sprintf("%s: receive from %s", prefix, tag))
					} else {
						l, nf := labelFromChan(ue.X)
						lbl = addPrefix(l, prefix+": receive from ")
						needFmt = nf
					}
					if needFmt {
						ensureImport("fmt")
					}
					if hasCtx {
						gated := opts.GuardDynamicLabels && needFmt
						valExpr := meta.value
						ensureImport("fmt")
						ptrLog := mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%%p", %s)) }`, exprString(ue.X)))
						blockStmts := []ast.Stmt{st}
						if valExpr != "" && (opts.Level == "regions_logs" || opts.Level == "") {
							blockStmts = append(blockStmts, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "value", fmt.Sprint(%s)) }`, valExpr)))
						}
						blk := &ast.BlockStmt{List: blockStmts}
						outs = append(outs, ptrLog)
						outs = append(outs, wrapWithRegionExprGuarded(lbl, blk, gated))
					} else {
						outs = append(outs, st)
					}
				} else {
					// Detect close(ch) calls and log channel close with pointer identity
					if ce, ok := s.X.(*ast.CallExpr); ok {
						if id, ok2 := ce.Fun.(*ast.Ident); ok2 && id.Name == "close" && len(ce.Args) == 1 {
							if hasCtx {
								ensureImport("fmt")
								ptrLog := mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "chan_close", fmt.Sprintf("ptr=%%p", %s)) }`, exprString(ce.Args[0])))
								// wrap in a small region for readability
								lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"chan.close\""}
								blk := &ast.BlockStmt{List: []ast.Stmt{ptrLog, st}}
								outs = append(outs, WrapWithRegionExpr(lbl, blk, false))
								break
							}
						}
					}
					// Fallback: leave other expression statements unchanged.
					outs = append(outs, st)
				}
			case *ast.AssignStmt:
				// x = <-ch  OR  x := <-ch
				if len(s.Rhs) == 1 {
					// Detect channel creation: x := make(chan T, cap)
					if ce, ok := s.Rhs[0].(*ast.CallExpr); ok {
						if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "make" && len(ce.Args) >= 1 {
							if ct, ok := ce.Args[0].(*ast.ChanType); ok {
								// We have a make(chan ...) expression. If we have ctx, emit a chan_make log right after the assignment.
								if hasCtx {
									ensureImport("fmt")
									// Use first LHS as the variable holding the channel pointer
									var lhsExpr string = ""
									if len(s.Lhs) > 0 {
										lhsExpr = exprString(s.Lhs[0])
									}
									// Compute cap argument string; default to 0 if not provided
									capExpr := "0"
									if len(ce.Args) >= 2 {
										capExpr = exprString(ce.Args[1])
									}
									// Render element type as a string for logs
									typeStr := exprString(ct.Value)
									// Append original statement, then the log expr
									outs = append(outs, st)
									logStmt := mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "chan_make", fmt.Sprintf("ptr=%%p cap=%%d type=%%s", %s, %s, %q)) }`, lhsExpr, capExpr, typeStr))
									outs = append(outs, logStmt)
									continue
								}
							}
						}
					}
					if ue, ok := s.Rhs[0].(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
						var lbl ast.Expr
						var needFmt bool
						prefix := "worker"
						if fnRole != "" {
							prefix = fnRole
						}
						if meta.recv != "" || (meta.label != "" && meta.send == "") {
							tag := meta.recv
							if tag == "" {
								tag = meta.label
							}
							lbl, needFmt = buildLabelExpr(fmt.Sprintf("%s: receive from %s", prefix, tag))
						} else {
							l, nf := labelFromChan(ue.X)
							lbl = addPrefix(l, prefix+": receive from ")
							needFmt = nf
						}
						if needFmt {
							ensureImport("fmt")
						}
						if hasCtx {
							gated := opts.GuardDynamicLabels && needFmt
							valExpr := meta.value
							if valExpr == "" && len(s.Lhs) > 0 {
								valExpr = exprString(s.Lhs[0])
							}
							ensureImport("fmt")
							ptrLog := mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%%p", %s)) }`, exprString(ue.X)))
							blockStmts := []ast.Stmt{st}
							if valExpr != "" && (opts.Level == "regions_logs" || opts.Level == "") {
								blockStmts = append(blockStmts, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx, "value", fmt.Sprint(%s)) }`, valExpr)))
							}
							blk := &ast.BlockStmt{List: blockStmts}
							outs = append(outs, ptrLog)
							outs = append(outs, wrapWithRegionExprGuarded(lbl, blk, gated))
						} else {
							outs = append(outs, st)
						}
						break
					}
					// I/O classification for calls on RHS
					if (enableJSON || enableDB || enableHTTP || enableOS || opts.AddIORegions) && (hasCtx || opts.IOAssumeBackground) {
						if ce, ok := s.Rhs[0].(*ast.CallExpr); ok {
							if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
								// encoding/json package functions
								if enableJSON {
									if selJSON, ok := isJSONCall(ce); ok {
										if name := jsonRegionLabel(selJSON.Sel.Name); name != "" {
											lbl := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", name)}
											var ctxExpr ast.Expr = ast.NewIdent("ctx")
											if !hasCtx && opts.IOAssumeBackground {
												ensureImport("context")
												ctxExpr = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Background")}}
											}
											if s.Tok == token.DEFINE {
												outs = append(outs, wrapWithStartEndCtxExpr(lbl, ctxExpr, st))
											} else {
												outs = append(outs, WrapWithRegionCtxExpr(ctxExpr, lbl, st, false))
											}
											break
										}
									}
								}
								// database/sql heuristics by method name when imported
								if enableDB && hasDBImport {
									if _, m, ok := isSQLSelector(se); ok {
										var ctxExpr ast.Expr = ast.NewIdent("ctx")
										if len(ce.Args) > 0 && (strings.HasSuffix(m, "Context") || strings.EqualFold(m, "BeginTx")) {
											ctxExpr = ce.Args[0]
										}
										label := "db.query"
										low := strings.ToLower(m)
										if strings.HasPrefix(low, "exec") {
											label = "db.exec"
										}
										if strings.HasPrefix(low, "begin") {
											label = "db.begin"
										}
										if asg, ok := st.(*ast.AssignStmt); ok && asg.Tok == token.DEFINE {
											outs = append(outs, wrapWithStartEndCtxExpr(&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", label)}, ctxExpr, st))
										} else {
											outs = append(outs, WrapWithRegionCtxExpr(ctxExpr, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", label)}, st, false))
										}
										break
									} else {
										low := strings.ToLower(se.Sel.Name)
										if strings.HasPrefix(low, "query") {
											outs = append(outs, WrapWithRegionExpr(&ast.BasicLit{Kind: token.STRING, Value: "\"db.query\""}, st, false))
											break
										}
										if strings.HasPrefix(low, "exec") {
											outs = append(outs, WrapWithRegionExpr(&ast.BasicLit{Kind: token.STRING, Value: "\"db.exec\""}, st, false))
											break
										}
										if strings.HasPrefix(low, "begin") {
											outs = append(outs, WrapWithRegionExpr(&ast.BasicLit{Kind: token.STRING, Value: "\"db.begin\""}, st, false))
											break
										}
									}
								}
								// net/http and os/io ioutil in assignment positions
								if enableHTTP || enableOS || opts.AddIORegions {
									low := strings.ToLower(se.Sel.Name)
									// http.Client.Do heuristic
									if enableHTTP && se.Sel.Name == "Do" {
										hasHTTP := false
										for _, p := range importPaths {
											if p == "net/http" {
												hasHTTP = true
												break
											}
										}
										if hasHTTP {
											lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"http.call\""}
											if s.Tok == token.DEFINE {
												outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
											} else {
												outs = append(outs, WrapWithRegionExpr(lbl, st, false))
											}
											break
										}
									}
									// http.Get/Post/Head/Do
									if enableHTTP {
										if xid, ok := se.X.(*ast.Ident); ok && xid.Name == "http" {
											switch low {
											case "get", "post", "head", "do":
												lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"http.call\""}
												if s.Tok == token.DEFINE {
													outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
												} else {
													outs = append(outs, WrapWithRegionExpr(lbl, st, false))
												}
											}
										}
									}
									// os.Open/OpenFile/ReadFile/WriteFile
									if enableOS {
										if xid, ok := se.X.(*ast.Ident); ok && xid.Name == "os" {
											switch se.Sel.Name {
											case "Open", "OpenFile":
												lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"file.open\""}
												if s.Tok == token.DEFINE {
													outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
												} else {
													outs = append(outs, WrapWithRegionExpr(lbl, st, false))
												}
											case "ReadFile":
												lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"file.readfile\""}
												if s.Tok == token.DEFINE {
													outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
												} else {
													outs = append(outs, WrapWithRegionExpr(lbl, st, false))
												}
											case "WriteFile":
												lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"file.writefile\""}
												if s.Tok == token.DEFINE {
													outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
												} else {
													outs = append(outs, WrapWithRegionExpr(lbl, st, false))
												}
											}
										}
										// io.Copy
										if xid, ok := se.X.(*ast.Ident); ok && xid.Name == "io" && se.Sel.Name == "Copy" {
											lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"io.copy\""}
											if s.Tok == token.DEFINE {
												outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
											} else {
												outs = append(outs, WrapWithRegionExpr(lbl, st, false))
											}
										}
										// ioutil.ReadFile
										if xid, ok := se.X.(*ast.Ident); ok && xid.Name == "ioutil" && se.Sel.Name == "ReadFile" {
											lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"file.readfile\""}
											if s.Tok == token.DEFINE {
												outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
											} else {
												outs = append(outs, WrapWithRegionExpr(lbl, st, false))
											}
										}
										// instance methods Read/Write
										if se.Sel.Name == "Read" || se.Sel.Name == "Write" {
											lbl := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"file.%s\"", strings.ToLower(se.Sel.Name))}
											if s.Tok == token.DEFINE {
												outs = append(outs, wrapWithStartEndCtxExpr(lbl, ast.NewIdent("ctx"), st))
											} else {
												outs = append(outs, WrapWithRegionExpr(lbl, st, false))
											}
										}
									}
								}
							}
						}
					}
				}
				outs = append(outs, st)
			case *ast.IfStmt:
				s.Body.List = instrumentStmts(s.Body.List, hasCtx, fnRole)
				if s.Else != nil {
					if b, ok := s.Else.(*ast.BlockStmt); ok {
						b.List = instrumentStmts(b.List, hasCtx, fnRole)
					}
				}
				outs = append(outs, st)

			case *ast.RangeStmt:
				if s.Body != nil {
					// Heuristic: inject loop variable capture copies into goroutine func literals
					var names []string
					if id, ok := s.Key.(*ast.Ident); ok && id.Name != "_" {
						names = append(names, id.Name)
					}
					if id, ok := s.Value.(*ast.Ident); ok && id.Name != "_" {
						names = append(names, id.Name)
					}
					injectCopiesInGoLiterals(s.Body, names)
					// Loop-level region: enabled if AddLoopRegions or explicit hint via gtv:loop=label
					if hasCtx && loopBodySafe(s.Body) && (opts.AddLoopRegions || meta.loop != "") {
						label := "loop:range"
						if meta.loop != "" {
							label = "loop:" + meta.loop
						}
						lbl := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", label)}
						orig := s.Body.List
						s.Body.List = []ast.Stmt{WrapWithRegionExpr(lbl, &ast.BlockStmt{List: orig}, false)}
					} else if hasCtx && meta.loop != "" && loopBodyForceable(s.Body, allowBareReturn) {
						// Forced loop region for range with break/continue (no return/goto/labelled branches)
						label := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", "loop:"+meta.loop)}
						// Declare flags in the loop body scope
						pre := []ast.Stmt{mkStmt("var __gtvBreak bool"), mkStmt("var __gtvCont bool")}
						if allowBareReturn {
							pre = append(pre, mkStmt("var __gtvReturn bool"))
						}
						// Transform body to set flags and return on break/continue
						transformed := rewriteBreakContinueBody(s.Body, "__gtvBreak", "__gtvCont", func() string {
							if allowBareReturn {
								return "__gtvReturn"
							}
							return ""
						}())
						// Wrap transformed block in a region
						wrapped := WrapWithRegionExpr(label, &ast.BlockStmt{List: transformed}, false)
						// After the call, act on flags
						post := []ast.Stmt{mkStmt("if __gtvBreak { break }"), mkStmt("if __gtvCont { continue }")}
						if allowBareReturn {
							post = append(post, mkStmt("if __gtvReturn { return }"))
						}
						s.Body.List = append(append(pre, wrapped), post...)
					} else {
						if meta.loop != "" && hasCtx {
							warnings = append(warnings, fmt.Sprintf("gtv:loop ignored at %s (body not safe and not forceable)", posString(s)))
						}
						s.Body.List = instrumentStmts(s.Body.List, hasCtx, fnRole)
					}
				}
				outs = append(outs, st)
			case *ast.ForStmt:
				if s.Body != nil {
					// Heuristic: try to detect a simple loop index and capture it
					var names []string
					switch po := s.Post.(type) {
					case *ast.IncDecStmt:
						if id, ok := po.X.(*ast.Ident); ok {
							names = append(names, id.Name)
						}
					case *ast.AssignStmt:
						for _, lhs := range po.Lhs {
							if id, ok := lhs.(*ast.Ident); ok {
								names = append(names, id.Name)
							}
						}
					}
					// Also consider short var declarations in init
					switch ini := s.Init.(type) {
					case *ast.AssignStmt:
						if ini.Tok == token.DEFINE {
							for _, lhs := range ini.Lhs {
								if id, ok := lhs.(*ast.Ident); ok {
									names = append(names, id.Name)
								}
							}
						}
					}
					injectCopiesInGoLiterals(s.Body, names)
					// Optionally wrap loop body in a region if safe (no break/continue/return/goto)
					if hasCtx && loopBodySafe(s.Body) && (opts.AddLoopRegions || meta.loop != "") {
						label := "loop:for"
						if meta.loop != "" {
							label = "loop:" + meta.loop
						}
						lbl := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", label)}
						orig := s.Body.List
						s.Body.List = []ast.Stmt{WrapWithRegionExpr(lbl, &ast.BlockStmt{List: orig}, false)}
					} else if hasCtx && meta.loop != "" && loopBodyForceable(s.Body, allowBareReturn) {
						// Forced loop region for classic for loop with break/continue (no return/goto/labelled branches)
						label := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", "loop:"+meta.loop)}
						pre := []ast.Stmt{mkStmt("var __gtvBreak bool"), mkStmt("var __gtvCont bool")}
						if allowBareReturn {
							pre = append(pre, mkStmt("var __gtvReturn bool"))
						}
						transformed := rewriteBreakContinueBody(s.Body, "__gtvBreak", "__gtvCont", func() string {
							if allowBareReturn {
								return "__gtvReturn"
							}
							return ""
						}())
						wrapped := WrapWithRegionExpr(label, &ast.BlockStmt{List: transformed}, false)
						post := []ast.Stmt{mkStmt("if __gtvBreak { break }"), mkStmt("if __gtvCont { continue }")}
						if allowBareReturn {
							post = append(post, mkStmt("if __gtvReturn { return }"))
						}
						s.Body.List = append(append(pre, wrapped), post...)
					} else {
						if meta.loop != "" && hasCtx {
							warnings = append(warnings, fmt.Sprintf("gtv:loop ignored at %s (body not safe and not forceable)", posString(s)))
						}
						s.Body.List = instrumentStmts(s.Body.List, hasCtx, fnRole)
					}
				}
				outs = append(outs, st)
			case *ast.SwitchStmt:
				if s.Body != nil {
					for _, cc := range s.Body.List {
						if c, ok := cc.(*ast.CaseClause); ok {
							c.Body = instrumentStmts(c.Body, hasCtx, fnRole)
						}
					}
				}
				outs = append(outs, st)
			case *ast.SelectStmt:
				hasDefault := selectHasDefault(s)
				needSelectLog := hasCtx && !hasDefault
				var selVar string
				if needSelectLog {
					ensureImport("fmt")
					ensureImport("time")
					selVar = fmt.Sprintf("__gtvSel%d", regCounter+1)
					regCounter++
				}
				if s.Body != nil {
					for _, cc := range s.Body.List {
						if c, ok := cc.(*ast.CommClause); ok {
							rewriteSelectClauseAssign(c)
							c.Body = instrumentStmts(c.Body, hasCtx, fnRole)
							appliedSelectRegion := false
							if hasCtx && c.Comm != nil {
								cmts := commentText(leadingCommentGroup(c))
								clauseMeta := parseAnn(cmts)
								kind, chExpr, valExpr := detectSelectComm(c.Comm, clauseMeta)
								if kind != "" && chExpr != nil && clauseBodySafe(&ast.BlockStmt{List: c.Body}) {
									lbl, needFmt := buildSendRecvLabel(kind, chExpr, clauseMeta, fnRole)
									if needFmt {
										ensureImport("fmt")
									}
									bodyCopy := make([]ast.Stmt, len(c.Body))
									copy(bodyCopy, c.Body)
									if kind == "recv" && valExpr != "" {
										ensureImport("fmt")
										bodyCopy = append(bodyCopy, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx,"value", fmt.Sprint(%s)) }`, valExpr)))
									}
									regionStmt := WrapWithRegionExpr(lbl, &ast.BlockStmt{List: bodyCopy}, opts.GuardDynamicLabels)
									ensureImport("fmt")
									chPtrStmt := mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx,"ch_ptr", fmt.Sprintf("ptr=%%p", %s)) }`, exprString(chExpr)))
									newBody := []ast.Stmt{chPtrStmt}
									if kind == "send" && valExpr != "" {
										ensureImport("fmt")
										newBody = append(newBody, mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(ctx,"value", fmt.Sprint(%s)) }`, valExpr)))
									}
									newBody = append(newBody, regionStmt)
									c.Body = newBody
									appliedSelectRegion = true
								}
							}
							if needSelectLog && c.Comm != nil {
								chosen := mkStmt(fmt.Sprintf("if trace.IsEnabled(){ trace.Log(ctx, \"select\", \"select_chosen id=\"+%s) }", selVar))
								c.Body = append([]ast.Stmt{chosen}, c.Body...)
							}
							if !appliedSelectRegion {
								wrapSelectReceive(c)
							}
						}
					}
				}
				if needSelectLog {
					sidDecl := mkStmt(fmt.Sprintf("%s := fmt.Sprintf(\"s%%v\", time.Now().UnixNano())", selVar))
					beginLog := mkStmt(fmt.Sprintf("if trace.IsEnabled(){ trace.Log(ctx, \"select\", \"select_begin id=\"+%s) }", selVar))
					endLog := mkStmt(fmt.Sprintf("if trace.IsEnabled(){ trace.Log(ctx, \"select\", \"select_end id=\"+%s) }", selVar))
					block := &ast.BlockStmt{List: []ast.Stmt{sidDecl, beginLog, st, endLog}}
					outs = append(outs, block)
				} else {
					outs = append(outs, st)
				}
			default:
				outs = append(outs, st)
			}
			// phase end marker after the statement
			if meta.phaseEnd != "" && hasCtx {
				if n := len(phaseStack); n > 0 {
					varName := phaseStack[n-1]
					phaseStack = phaseStack[:n-1]
					outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(varName), Sel: ast.NewIdent("End")}}})
					// optional end log
					outs = append(outs, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("Log")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.BasicLit{Kind: token.STRING, Value: "\"main\""}, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", meta.phaseEnd)}}}})
				}
			}
		}
		return outs
	}

	for _, d := range file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Honor only/skip sets for statement-level instrumentation
		if len(onlySet) > 0 {
			if !onlySet[fn.Name.Name] {
				continue
			}
		}
		if skipSet[fn.Name.Name] {
			continue
		}
		// Determine if ctx is in scope for this function
		hasCtx := false
		if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
				if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
					hasCtx = true
				}
			}
		}
		// Determine function-level role from doc (again, lightweight)
		fRole := ""
		if fn.Doc != nil {
			for _, c := range fn.Doc.List {
				txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
				if strings.HasPrefix(txt, "gtv:role=") {
					fRole = strings.TrimSpace(strings.TrimPrefix(txt, "gtv:role="))
					fRole = strings.Trim(fRole, "`\"")
				}
			}
		}
		// Determine if bare return is legal in this function (zero results or named results)
		allowBareReturn = false
		if fn.Type != nil && fn.Type.Results != nil {
			// named results permitted for bare return
			named := true
			for _, fld := range fn.Type.Results.List {
				if len(fld.Names) == 0 {
					named = false
					break
				}
			}
			allowBareReturn = named
		} else {
			// no results -> bare return ok
			allowBareReturn = true
		}
		if opts.Level != "tasks_only" {
			fn.Body.List = instrumentStmts(fn.Body.List, hasCtx, fRole)
		}
	}

	// Goroutine instrumentation: ensure goroutine targets accept ctx and call sites pass ctx.
	// We only transform simple cases:
	//   - go f(...) where f is a top-level function in this file (Ident)
	//   - go func(){...}() where we add a ctx parameter and pass ctx in the call
	// We do not modify methods (SelectorExpr) or external functions.

	// Collect all functions by name for quick lookup.
	funs := make(map[string]*ast.FuncDecl)
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name != nil {
			funs[fn.Name.Name] = fn
		}
	}

	// Discover goroutine targets.
	needCtxFor := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		gs, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		ce := gs.Call
		if ce == nil {
			return true
		}
		switch fun := ce.Fun.(type) {
		case *ast.Ident:
			if _, ok := funs[fun.Name]; ok {
				needCtxFor[fun.Name] = true
			}
		case *ast.FuncLit:
			// add ctx param to literal if not present, and pass ctx
			// params: (ctx context.Context)
			if fun.Type.Params == nil {
				fun.Type.Params = &ast.FieldList{}
			}
			// add only if not already present as first param
			hasCtx := false
			if len(fun.Type.Params.List) > 0 {
				if se, ok := fun.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
					if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
						hasCtx = true
					}
				}
			}
			if !hasCtx {
				fun.Type.Params.List = append([]*ast.Field{{
					Names: []*ast.Ident{ast.NewIdent("ctx")},
					Type:  &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Context")},
				}}, fun.Type.Params.List...)
			}
			// ensure call has ctx argument
			if !(len(ce.Args) > 0 && isCtxIdent(ce.Args[len(ce.Args)-1])) {
				ce.Args = append(ce.Args, ast.NewIdent("ctx"))
			}
			// Optional goroutine region wrapping for func literals
			if opts.AddGoroutineRegions {
				// Build label: "goroutine: anon"
				lbl := &ast.BasicLit{Kind: token.STRING, Value: "\"goroutine: anon\""}
				// Wrap the entire body list into a single WithRegion; keep existing statements
				if fun.Body != nil {
					block := &ast.BlockStmt{List: fun.Body.List}
					wrapped := WrapWithRegionExpr(lbl, block, false)
					fun.Body.List = []ast.Stmt{wrapped}
				}
			}
		}
		return true
	})

	// For each named function used in a goroutine, ensure it accepts ctx.
	for name := range needCtxFor {
		fn := funs[name]
		if fn == nil {
			continue
		}
		hasCtx := false
		if fn.Type.Params != nil && len(fn.Type.Params.List) > 0 {
			if se, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok {
				if id, ok := se.X.(*ast.Ident); ok && id.Name == "context" && se.Sel.Name == "Context" {
					hasCtx = true
				}
			}
		}
		if !hasCtx {
			fld := &ast.Field{Names: []*ast.Ident{ast.NewIdent("ctx")}, Type: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("Context")}}
			if fn.Type.Params == nil {
				fn.Type.Params = &ast.FieldList{}
			}
			fn.Type.Params.List = append([]*ast.Field{fld}, fn.Type.Params.List...)
		}
	}

	// Update call sites to pass ctx for those functions.
	ast.Inspect(file, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		if !needCtxFor[id.Name] {
			return true
		}
		// If first arg already ctx, leave. Otherwise prepend ctx.
		if len(ce.Args) > 0 && isCtxIdent(ce.Args[0]) {
			return true
		}
		ce.Args = append([]ast.Expr{ast.NewIdent("ctx")}, ce.Args...)
		return true
	})

	// Optional: wrap named function goroutines in a region as well.
	if opts.AddGoroutineRegions {
		ast.Inspect(file, func(n ast.Node) bool {
			gs, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			ce := gs.Call
			if ce == nil {
				return true
			}
			if id, ok := ce.Fun.(*ast.Ident); ok {
				// Replace go f(args...) with go func(){ trace.WithRegion(ctx, "goroutine: f", func(){ f(args...) }) }()
				callCopy := *ce
				name := id.Name
				lbl := &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"goroutine: %s\"", name)}
				body := &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &callCopy}}}
				wrapped := WrapWithRegionExpr(lbl, body, false)
				lit := &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{wrapped}}}
				gs.Call = &ast.CallExpr{Fun: lit, Args: []ast.Expr{}}
			}
			return true
		})
	}

	// Note: method goroutines are already wrapped in instrumentStmts for wg.Done(). We do not
	// add a ctx parameter to methods automatically; they can capture ctx from closure.

	// If we used a WaitGroup, declare it at the top of Run<Name>Program and wait on exit.
	if usedWG && runFn != nil {
		ensureImport("sync")
		// var wg sync.WaitGroup; defer wg.Wait()
		varWG := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("wg")}, Type: &ast.SelectorExpr{X: ast.NewIdent("sync"), Sel: ast.NewIdent("WaitGroup")}}}}}
		deferWait := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("wg"), Sel: ast.NewIdent("Wait")}}}
		runFn.Body.List = append([]ast.Stmt{varWG, deferWait}, runFn.Body.List...)
	}

	// If timeout is requested, add WithTimeout at the very top (before task creation)
	if hasTimeout && runFn != nil {
		ensureImport("time")
		// ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutNS)); defer cancel()
		assign := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("cancel")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("context"), Sel: ast.NewIdent("WithTimeout")}, Args: []ast.Expr{ast.NewIdent("ctx"), &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("time"), Sel: ast.NewIdent("Duration")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", timeoutNS)}}}}}}}
		deferCancel := &ast.DeferStmt{Call: &ast.CallExpr{Fun: ast.NewIdent("cancel")}}
		runFn.Body.List = append([]ast.Stmt{assign, deferCancel}, runFn.Body.List...)
	}

	// If a done channel is requested, declare and close it.
	if doneName != "" && runFn != nil {
		// done := make(chan struct{}); defer close(done)
		makeChan := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{&ast.ChanType{Value: &ast.StructType{Fields: &ast.FieldList{}}}}}
		assign := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(doneName)}, Tok: token.DEFINE, Rhs: []ast.Expr{makeChan}}
		deferClose := &ast.DeferStmt{Call: &ast.CallExpr{Fun: ast.NewIdent("close"), Args: []ast.Expr{ast.NewIdent(doneName)}}}
		runFn.Body.List = append([]ast.Stmt{assign, deferClose}, runFn.Body.List...)
	}

	ensureChPtrLogs(file, ensureImport)

	// Add spawn counter declaration if any goroutines were instrumented.
	if usedSpawn {
		ensureImport("sync/atomic")
		spawnDecl := &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ast.NewIdent("gtvSpawnID")},
				Type:  ast.NewIdent("uint64"),
			}},
		}
		insertIdx := 0
		for i, d := range file.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
				insertIdx = i + 1
			}
		}
		file.Decls = append(file.Decls[:insertIdx], append([]ast.Decl{spawnDecl}, file.Decls[insertIdx:]...)...)
	}

	// Add init() that registers the workload name so live/offline runners can dispatch dynamically.
	reg := &ast.FuncDecl{
		Name: ast.NewIdent("init"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		// Call RegisterWorkload in the same package (no qualifier).
		Body: &ast.BlockStmt{List: []ast.Stmt{mkExpr(fmt.Sprintf(`RegisterWorkload("%s", %s)`, strings.ToLower(name), runName))}},
	}
	file.Decls = append(file.Decls, reg)

	// Print the file
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return nil, err
	}
	// Emit warnings to stderr (best-effort)
	if len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, w)
		}
	}
	return buf.Bytes(), nil
}

func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range cg.List {
		b.WriteString(c.Text)
		b.WriteString("\n")
	}
	return b.String()
}

// loopBodySafe reports whether a loop body is safe to wrap in a closure without
// changing semantics. We conservatively return false if the body contains
// break/continue/goto/return.
func loopBodySafe(b *ast.BlockStmt) bool {
	if b == nil {
		return false
	}
	safe := true
	ast.Inspect(b, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.BranchStmt:
			if t.Tok == token.BREAK || t.Tok == token.CONTINUE || t.Tok == token.GOTO {
				safe = false
				return false
			}
		case *ast.ReturnStmt:
			safe = false
			return false
		}
		return true
	})
	return safe
}

// clauseBodySafe reports whether a select clause body is safe to wrap
// in a synthetic region without altering control flow semantics.
// We conservatively reject bodies containing break/continue/goto/return.
func clauseBodySafe(b *ast.BlockStmt) bool {
	if b == nil {
		return false
	}
	safe := true
	ast.Inspect(b, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.BranchStmt:
			if t.Tok == token.BREAK || t.Tok == token.CONTINUE || t.Tok == token.GOTO {
				safe = false
				return false
			}
		case *ast.ReturnStmt:
			safe = false
			return false
		}
		return true
	})
	return safe
}

// loopBodyForceable reports whether we can apply a forced loop region by
// rewriting break/continue (unlabelled) into flags + return inside an inner closure.
// We conservatively reject bodies containing return/goto or any labelled branch.
func loopBodyForceable(b *ast.BlockStmt, allowReturn bool) bool {
	if b == nil {
		return false
	}
	ok := true
	ast.Inspect(b, func(n ast.Node) bool {
		switch t := n.(type) {
		case *ast.ReturnStmt:
			if !allowReturn {
				ok = false
				return false
			}
		case *ast.BranchStmt:
			if t.Tok == token.GOTO {
				ok = false
				return false
			}
			if t.Label != nil {
				ok = false
				return false
			}
		}
		// Do not descend into nested loops; branch inside nested loops should not affect outer flags.
		switch n.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return false
		}
		return true
	})
	return ok
}

// rewriteBreakContinueBody returns a new list of statements where unlabelled
// break/continue in the provided body are replaced with `brFlag=true; return`
// and `contFlag=true; return` respectively. It avoids descending into nested
// loops so that inner loops keep their semantics.
func rewriteBreakContinueBody(body *ast.BlockStmt, brFlag, contFlag, retFlag string) []ast.Stmt {
	if body == nil {
		return nil
	}
	var rewriteList func([]ast.Stmt) []ast.Stmt
	rewriteList = func(list []ast.Stmt) []ast.Stmt {
		outs := make([]ast.Stmt, 0, len(list))
		for _, st := range list {
			switch s := st.(type) {
			case *ast.BranchStmt:
				if s.Label == nil {
					switch s.Tok {
					case token.BREAK:
						// { brFlag = true; return }
						outs = append(outs, &ast.BlockStmt{List: []ast.Stmt{
							&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(brFlag)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent("true")}},
							&ast.ReturnStmt{},
						}})
						continue
					case token.CONTINUE:
						outs = append(outs, &ast.BlockStmt{List: []ast.Stmt{
							&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(contFlag)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent("true")}},
							&ast.ReturnStmt{},
						}})
						continue
					}
				}
				outs = append(outs, st)
			case *ast.ReturnStmt:
				if retFlag != "" {
					outs = append(outs, &ast.BlockStmt{List: []ast.Stmt{
						&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(retFlag)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent("true")}},
						&ast.ReturnStmt{},
					}})
					continue
				}
				outs = append(outs, st)
			case *ast.BlockStmt:
				s.List = rewriteList(s.List)
				outs = append(outs, s)
			case *ast.IfStmt:
				s.Body.List = rewriteList(s.Body.List)
				if b, ok := s.Else.(*ast.BlockStmt); ok {
					b.List = rewriteList(b.List)
					s.Else = b
				}
				outs = append(outs, s)
			case *ast.SwitchStmt:
				if s.Body != nil {
					for _, cc := range s.Body.List {
						if c, ok := cc.(*ast.CaseClause); ok {
							c.Body = rewriteList(c.Body)
						}
					}
				}
				outs = append(outs, s)
			case *ast.SelectStmt:
				if s.Body != nil {
					for _, cc := range s.Body.List {
						if c, ok := cc.(*ast.CommClause); ok {
							c.Body = rewriteList(c.Body)
						}
					}
				}
				outs = append(outs, s)
			case *ast.ForStmt, *ast.RangeStmt:
				// Do not descend; preserve inner loop semantics
				outs = append(outs, st)
			default:
				outs = append(outs, st)
			}
		}
		return outs
	}
	return rewriteList(body.List)
}

// (region wrappers moved to astutil.go)

func selectHasDefault(s *ast.SelectStmt) bool {
	if s == nil || s.Body == nil {
		return false
	}
	for _, cc := range s.Body.List {
		if c, ok := cc.(*ast.CommClause); ok && c.Comm == nil {
			return true
		}
	}
	return false
}

func isCtxIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "ctx"
}

func ensureChPtrLogs(file *ast.File, ensureImport func(string)) {
	changed := false
	var visit func([]ast.Stmt) []ast.Stmt
	visit = func(stmts []ast.Stmt) []ast.Stmt {
		for i := 0; i < len(stmts); i++ {
			stmts[i] = ensureChPtrInStmt(stmts[i], visit)
			if logStmt := chPtrBeforeRegion(stmts, i); logStmt != nil {
				stmts = append(stmts[:i], append([]ast.Stmt{logStmt}, stmts[i:]...)...)
				changed = true
				i++
			}
		}
		return stmts
	}
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Body != nil {
			fn.Body.List = visit(fn.Body.List)
		}
	}
	if changed {
		ensureImport("fmt")
	}
}

func ensureChPtrInStmt(stmt ast.Stmt, visit func([]ast.Stmt) []ast.Stmt) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		s.List = visit(s.List)
	case *ast.IfStmt:
		if s.Body != nil {
			s.Body.List = visit(s.Body.List)
		}
		switch els := s.Else.(type) {
		case *ast.BlockStmt:
			els.List = visit(els.List)
		case *ast.IfStmt:
			ensureChPtrInStmt(els, visit)
		}
	case *ast.ForStmt:
		if s.Body != nil {
			s.Body.List = visit(s.Body.List)
		}
	case *ast.RangeStmt:
		if s.Body != nil {
			s.Body.List = visit(s.Body.List)
		}
	case *ast.SwitchStmt:
		if s.Body != nil {
			for _, cc := range s.Body.List {
				if c, ok := cc.(*ast.CaseClause); ok {
					c.Body = visit(c.Body)
				}
			}
		}
	case *ast.TypeSwitchStmt:
		if s.Body != nil {
			for _, cc := range s.Body.List {
				if c, ok := cc.(*ast.CaseClause); ok {
					c.Body = visit(c.Body)
				}
			}
		}
	case *ast.SelectStmt:
		if s.Body != nil {
			for _, cc := range s.Body.List {
				if c, ok := cc.(*ast.CommClause); ok {
					c.Body = visit(c.Body)
				}
			}
		}
	}
	return stmt
}

func chPtrBeforeRegion(stmts []ast.Stmt, idx int) ast.Stmt {
	if idx < 0 || idx >= len(stmts) {
		return nil
	}
	if idx > 0 && isChPtrLog(stmts[idx-1]) {
		return nil
	}
	call := regionCallFromStmt(stmts[idx])
	if call == nil {
		return nil
	}
	chExpr := channelExprFromRegion(call)
	if chExpr == nil {
		return nil
	}
	return EmitChPtrLog(chExpr)
}

func regionCallFromStmt(stmt ast.Stmt) *ast.CallExpr {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		return regionCallFromExpr(s.X)
	case *ast.IfStmt:
		if len(s.Body.List) == 1 {
			if inner := regionCallFromStmt(s.Body.List[0]); inner != nil {
				return inner
			}
		}
	}
	return nil
}

func regionCallFromExpr(expr ast.Expr) *ast.CallExpr {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "trace" || sel.Sel.Name != "WithRegion" {
		return nil
	}
	return call
}

func isChPtrLog(stmt ast.Stmt) bool {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "trace" || sel.Sel.Name != "Log" {
		return false
	}
	if len(call.Args) < 3 {
		return false
	}
	if lit, ok := call.Args[1].(*ast.BasicLit); ok {
		return lit.Value == "\"ch_ptr\""
	}
	return false
}

func channelExprFromRegion(call *ast.CallExpr) ast.Expr {
	if len(call.Args) < 3 {
		return nil
	}
	fl, ok := call.Args[2].(*ast.FuncLit)
	if !ok || fl.Body == nil || len(fl.Body.List) == 0 {
		return nil
	}
	first := fl.Body.List[0]
	for {
		if blk, ok := first.(*ast.BlockStmt); ok && len(blk.List) > 0 {
			first = blk.List[0]
			continue
		}
		break
	}
	switch stmt := first.(type) {
	case *ast.SendStmt:
		return stmt.Chan
	case *ast.ExprStmt:
		if ue, ok := stmt.X.(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
			return ue.X
		}
	case *ast.AssignStmt:
		if len(stmt.Rhs) == 1 {
			if ue, ok := stmt.Rhs[0].(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
				return ue.X
			}
		}
	}
	return nil
}
