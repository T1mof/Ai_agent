package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"ai_agent/internal/types"
)

type ASTMissingCloseAnalyzer struct{}

func NewASTMissingCloseAnalyzer() Analyzer {
	return &ASTMissingCloseAnalyzer{}
}

func (a *ASTMissingCloseAnalyzer) Name() string {
	return "ast_missing_close"
}

func (a *ASTMissingCloseAnalyzer) Run(projectRoot string) ([]types.Finding, error) {
	var out []types.Finding
	fset := token.NewFileSet()

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules" {
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

		relPath := normalizeProjectPath(absRoot, path)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			hasClose := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				deferStmt, ok := n.(*ast.DeferStmt)
				if !ok {
					return true
				}
				sel, ok := deferStmt.Call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Close" {
					hasClose = true
					return false
				}
				return true
			})

			if hasClose {
				continue
			}

			for _, stmt := range fn.Body.List {
				assign, ok := stmt.(*ast.AssignStmt)
				if !ok || len(assign.Rhs) != 1 {
					continue
				}

				call, ok := assign.Rhs[0].(*ast.CallExpr)
				if !ok {
					continue
				}

				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					continue
				}

				x, ok := sel.X.(*ast.Ident)
				if !ok || x.Name != "os" || sel.Sel.Name != "Open" {
					continue
				}

				pos := fset.Position(assign.Pos())
				out = append(out, types.Finding{
					ID:          "ast_missing_close:" + relPath + ":" + strconv.Itoa(pos.Line),
					Tool:        "ast",
					File:        relPath,
					Line:        pos.Line,
					Category:    "resource_management",
					Priority:    "P1",
					FixCategory: "A",
					Message:     "os.Open without defer Close in function",
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return out, nil
}
