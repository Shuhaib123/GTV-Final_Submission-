package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func exprString(fset *token.FileSet, e ast.Expr) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	_ = printer.Fprint(&b, fset, e)
	return b.String()
}

func labelFromChan(fset *token.FileSet, e ast.Expr) string {
	// clientOut[i] -> clientout[i], join -> join
	switch x := e.(type) {
	case *ast.Ident:
		return strings.ToLower(x.Name)
	case *ast.SelectorExpr:
		return strings.ToLower(x.Sel.Name)
	case *ast.IndexExpr:
		base := labelFromChan(fset, x.X)
		idx := exprString(fset, x.Index)
		return fmt.Sprintf("%s[%s]", base, idx)
	default:
		return "chan"
	}
}

type ins struct {
	line int
	text string
}

func main() {
	in := flag.String("in", "", "input Go file")
	dir := flag.String("dir", "", "walk a directory and annotate all .go files")
	out := flag.String("out", "", "output file (default: overwrite input)")
	flag.Parse()
	if *in == "" && *dir == "" {
		fmt.Fprintln(os.Stderr, "-in or -dir required")
		os.Exit(2)
	}
	if *dir != "" {
		// Walk directory mode
		runDir(*dir)
		return
	}
	if err := annotateFile(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDir(root string) {
	var files []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" || info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		// skip generated files by convention
		if strings.HasSuffix(p, "_gen.go") {
			return nil
		}
		files = append(files, p)
		return nil
	})
	for _, f := range files {
		// annotate each file in place
		if err := annotateFile(f, ""); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}

func annotateFile(in, out string) error {
	fset := token.NewFileSet()
	src, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	file, err := parser.ParseFile(fset, in, src, parser.ParseComments)
	if err != nil {
		return err
	}
	// prepare source lines for idempotency checks
	srcStr := string(src)
	lines := strings.Split(srcStr, "\n")
	var inserts []ins
	ast.Inspect(file, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.SendStmt:
			pos := fset.Position(s.Pos())
			lbl := labelFromChan(fset, s.Chan)
			val := exprString(fset, s.Value)
			txt := fmt.Sprintf("// gtv:send=%s,value=%s", lbl, val)
			inserts = append(inserts, ins{line: pos.Line, text: txt})
		case *ast.AssignStmt:
			if len(s.Rhs) == 1 {
				if ue, ok := s.Rhs[0].(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
					pos := fset.Position(s.Pos())
					lbl := labelFromChan(fset, ue.X)
					val := ""
					if len(s.Lhs) > 0 {
						val = exprString(fset, s.Lhs[0])
					}
					if val != "" {
						inserts = append(inserts, ins{line: pos.Line, text: fmt.Sprintf("// gtv:recv=%s,value=%s", lbl, val)})
					} else {
						inserts = append(inserts, ins{line: pos.Line, text: fmt.Sprintf("// gtv:recv=%s", lbl)})
					}
				}
			}
		case *ast.ExprStmt:
			if ue, ok := s.X.(*ast.UnaryExpr); ok && ue.Op == token.ARROW {
				pos := fset.Position(s.Pos())
				lbl := labelFromChan(fset, ue.X)
				inserts = append(inserts, ins{line: pos.Line, text: fmt.Sprintf("// gtv:recv=%s", lbl)})
			}
		case *ast.ForStmt:
			// Add loop hint above classic for loops if not already present
			pos := fset.Position(s.Pos())
			prev := pos.Line - 1
			if prev >= 1 {
				prevText := strings.TrimSpace(lines[prev-1])
				if strings.Contains(prevText, "gtv:loop") {
					return true
				}
			}
			inserts = append(inserts, ins{line: pos.Line, text: "// gtv:loop=for"})
		case *ast.RangeStmt:
			// Add loop hint above range loops if not already present
			pos := fset.Position(s.Pos())
			prev := pos.Line - 1
			if prev >= 1 {
				prevText := strings.TrimSpace(lines[prev-1])
				if strings.Contains(prevText, "gtv:loop") {
					return true
				}
			}
			inserts = append(inserts, ins{line: pos.Line, text: "// gtv:loop=range"})
		}
		return true
	})
	if len(inserts) == 0 {
		if out == "" || out == in {
			return nil
		}
		return os.WriteFile(out, src, 0644)
	}
	sort.Slice(inserts, func(i, j int) bool { return inserts[i].line < inserts[j].line })
	merged := make(map[int][]string)
	for _, it := range inserts {
		merged[it.line] = append(merged[it.line], it.text)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(src)))
	scanner.Split(bufio.ScanLines)
	var outBuf strings.Builder
	line := 0
	for scanner.Scan() {
		line++
		if cmts, ok := merged[line]; ok {
			for _, c := range cmts {
				outBuf.WriteString(c)
				outBuf.WriteByte('\n')
			}
		}
		outBuf.WriteString(scanner.Text())
		outBuf.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	dst := out
	if dst == "" {
		dst = in
	}
	return os.WriteFile(dst, []byte(outBuf.String()), 0644)
}
