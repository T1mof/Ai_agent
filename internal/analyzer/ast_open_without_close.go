package analyzer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"

	"ai_agent/internal/types"
)

type ASTOpenWithoutCloseAnalyzer struct{}

func NewASTOpenWithoutCloseAnalyzer() Analyzer {
	return &ASTOpenWithoutCloseAnalyzer{}
}

func (a *ASTOpenWithoutCloseAnalyzer) Name() string {
	return "ast_open_without_close"
}

func (a *ASTOpenWithoutCloseAnalyzer) Run(projectRoot string) ([]types.Finding, error) {
	var out []types.Finding
	fset := token.NewFileSet()

	err := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			closedVars := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				deferStmt, ok := n.(*ast.DeferStmt)
				if !ok {
					return true
				}

				call, ok := deferStmt.Call.Fun.(*ast.SelectorExpr)
				if !ok || call.Sel.Name != "Close" {
					return true
				}

				if ident, ok := call.X.(*ast.Ident); ok {
					closedVars[ident.Name] = true
				}
				return true
			})

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}

				for i, rhs := range assign.Rhs {
					call, ok := rhs.(*ast.CallExpr)
					if !ok {
						continue
					}

					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					pkg, ok := sel.X.(*ast.Ident)
					if !ok {
						continue
					}

					if pkg.Name != "os" {
						continue
					}
					if sel.Sel.Name != "Open" && sel.Sel.Name != "OpenFile" && sel.Sel.Name != "Create" {
						continue
					}

					if i >= len(assign.Lhs) {
						continue
					}

					target, ok := assign.Lhs[i].(*ast.Ident)
					if !ok || target.Name == "_" {
						continue
					}
					if closedVars[target.Name] {
						continue
					}

					pos := fset.Position(assign.Pos())
					out = append(out, types.Finding{
						ID:          fmt.Sprintf("open-without-close:%s:%d", path, pos.Line),
						Tool:        "ast",
						File:        path,
						Line:        pos.Line,
						Category:    "resource_management",
						Priority:    "P1",
						FixCategory: "C",
						Message:     "resource opened without defer Close()",
						Symbol:      target.Name,
					})
				}

				return true
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return out, nil
}
