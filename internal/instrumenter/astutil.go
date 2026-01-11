package instrumenter

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// EnsureImport adds an import to the file if not present.
func EnsureImport(file *ast.File, path string) {
	// already present?
	for _, im := range file.Imports {
		if strings.Trim(im.Path.Value, "\"") == path {
			return
		}
	}
	// find existing import decl
	var impDecl *ast.GenDecl
	for _, d := range file.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			impDecl = gd
			break
		}
	}
	spec := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", path)}}
	if impDecl != nil {
		impDecl.Specs = append(impDecl.Specs, spec)
	} else {
		impDecl = &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{spec}}
		file.Decls = append([]ast.Decl{impDecl}, file.Decls...)
	}
	file.Imports = append(file.Imports, spec)
}

func EnsureImports(file *ast.File, paths ...string) {
	for _, p := range paths {
		EnsureImport(file, p)
	}
}

func mkExpr(code string) ast.Stmt {
	e, err := parser.ParseExpr(code)
	if err == nil && e != nil {
		return &ast.ExprStmt{X: e}
	}
	return mkStmt(code)
}

func mkStmt(code string) ast.Stmt {
	fset := token.NewFileSet()
	src := "package p\nfunc _(){\n" + code + "\n}"
	f, err := parser.ParseFile(fset, "snippet.go", src, 0)
	if err != nil || len(f.Decls) == 0 {
		return &ast.EmptyStmt{}
	}
	fn := f.Decls[0].(*ast.FuncDecl)
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return &ast.EmptyStmt{}
	}
	return fn.Body.List[0]
}

func buildLabelExpr(label string) (ast.Expr, bool) {
	vars := make([]string, 0)
	var b strings.Builder
	for i := 0; i < len(label); i++ {
		ch := label[i]
		if ch == '[' {
			j := i + 1
			for j < len(label) && label[j] != ']' {
				j++
			}
			if j < len(label) && j > i+1 {
				ident := label[i+1 : j]
				if isIdent(ident) {
					b.WriteString("[%v]")
					vars = append(vars, ident)
					i = j
					continue
				}
			}
		}
		b.WriteByte(ch)
	}
	if len(vars) == 0 {
		return &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", label)}, false
	}
	fmtSel := &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Sprintf")}
	args := []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("\"%s\"", b.String())}}
	for _, v := range vars {
		args = append(args, ast.NewIdent(v))
	}
	return &ast.CallExpr{Fun: fmtSel, Args: args}, true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func addPrefix(lbl ast.Expr, prefix string) ast.Expr {
	return &ast.BinaryExpr{
		X:  &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", prefix)},
		Op: token.ADD,
		Y:  lbl,
	}
}

func AddPrefix(lbl ast.Expr, prefix string) ast.Expr {
	return addPrefix(lbl, prefix)
}

// Region wrappers
func WrapWithRegionExpr(label ast.Expr, stmt ast.Stmt, gated bool) ast.Stmt {
	// if trace.IsEnabled(){ trace.WithRegion(ctx, <label>, func(){ <stmt> }) } else { <stmt> }
	call := &ast.CallExpr{
		Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("WithRegion")},
		Args: []ast.Expr{ast.NewIdent("ctx"), label,
			&ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}},
		},
	}
	if !gated {
		return &ast.ExprStmt{X: call}
	}
	return &ast.IfStmt{
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("IsEnabled")}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{stmt}},
	}
}

func WrapWithRegionCtxExpr(ctx ast.Expr, label ast.Expr, stmt ast.Stmt, gated bool) ast.Stmt {
	call := &ast.CallExpr{
		Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("WithRegion")},
		Args: []ast.Expr{ctx, label,
			&ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}},
		},
	}
	if !gated {
		return &ast.ExprStmt{X: call}
	}
	return &ast.IfStmt{
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("IsEnabled")}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{stmt}},
	}
}

func WrapWithStartEndCtxExpr(ctx ast.Expr, label ast.Expr, body ast.Stmt, gated bool) ast.Stmt {
	// r := trace.StartRegion(ctx, label); defer r.End(); <body>
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("__jspt_reg_0")},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("StartRegion")},
			Args: []ast.Expr{ctx, label}}},
	}
	deferEnd := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("__jspt_reg_0"), Sel: ast.NewIdent("End")}}}
	block := &ast.BlockStmt{List: []ast.Stmt{assign, deferEnd, body}}
	if !gated {
		return block
	}
	return &ast.IfStmt{
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("trace"), Sel: ast.NewIdent("IsEnabled")}},
		Body: block,
		Else: &ast.BlockStmt{List: []ast.Stmt{body}},
	}
}

// Channel identity / lifecycle logs (centralized)

func EmitChPtrLog(expr ast.Expr) ast.Stmt {
	return EmitChPtrLogWithCtx(expr, "ctx")
}

func EmitChPtrLogWithCtx(expr ast.Expr, ctxName string) ast.Stmt {
	if ctxName == "" {
		return &ast.EmptyStmt{}
	}
	// if trace.IsEnabled(){ trace.Log(ctx, "ch_ptr", fmt.Sprintf("ptr=%p", EXPR)) }
	return mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(%s,"ch_ptr", fmt.Sprintf("ptr=%%p", %s)) }`, ctxName, sprint(expr)))
}

func EmitChNameLog(ch ast.Expr, name ast.Expr, ctxName string) ast.Stmt {
	if ch == nil || name == nil || ctxName == "" {
		return &ast.EmptyStmt{}
	}
	return mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(%s,"ch_name", fmt.Sprintf("ptr=%%p name=%%s", %s, %s)) }`, ctxName, sprint(ch), sprint(name)))
}

func EmitChanMakeLog(lhs ast.Expr, capExpr ast.Expr, typStr string, ctxName string) ast.Stmt {
	if ctxName == "" {
		return &ast.EmptyStmt{}
	}
	return mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(%s,"chan_make", fmt.Sprintf("lhs=%s typ=%s cap=%%v", %s)) }`,
		ctxName, sprint(lhs), typStr, sprint(capExpr)))
}

func EmitChanCloseLog(expr ast.Expr, ctxName string) ast.Stmt {
	if ctxName == "" {
		return &ast.EmptyStmt{}
	}
	return mkStmt(fmt.Sprintf(`if trace.IsEnabled(){ trace.Log(%s,"chan_close", fmt.Sprintf("ptr=%%p", %s)) }`, ctxName, sprint(expr)))
}

// helper: print expr to string (best-effort)
func sprint(e ast.Expr) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), e)
	return b.String()
}
