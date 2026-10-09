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

type ASTIgnoredErrorAnalyzer struct{}

func NewASTIgnoredErrorAnalyzer() Analyzer {
	return &ASTIgnoredErrorAnalyzer{}
}

func (a *ASTIgnoredErrorAnalyzer) Name() string {
	return "ast_ignored_error"
}

func (a *ASTIgnoredErrorAnalyzer) Run(projectRoot string) ([]types.Finding, error) {
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

		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}

			if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}

			ident, ok := assign.Lhs[0].(*ast.Ident)
			if !ok || ident.Name != "_" {
				return true
			}

			_, ok = assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				return true
			}

			pos := fset.Position(assign.Pos())
			out = append(out, types.Finding{
				ID:          "ast_ignored_error:" + relPath + ":" + strconv.Itoa(pos.Line),
				Tool:        "ast",
				File:        relPath,
				Line:        pos.Line,
				Category:    "error_handling",
				Priority:    "P1",
				FixCategory: "A",
				Message:     "possible ignored call result assigned to _",
			})

			return true
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return out, nil
}
