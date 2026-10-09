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

type ASTWeakErrHandlerAnalyzer struct{}

func NewASTWeakErrHandlerAnalyzer() Analyzer {
	return &ASTWeakErrHandlerAnalyzer{}
}

func (a *ASTWeakErrHandlerAnalyzer) Name() string {
	return "ast_weak_err_handler"
}

func (a *ASTWeakErrHandlerAnalyzer) Run(projectRoot string) ([]types.Finding, error) {
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
			ifs, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}

			if !isErrCheck(ifs) {
				return true
			}

			if len(ifs.Body.List) != 1 {
				return true
			}

			exprStmt, ok := ifs.Body.List[0].(*ast.ExprStmt)
			if !ok {
				return true
			}

			call, ok := exprStmt.X.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if (x.Name == "log" || x.Name == "fmt") && strings.HasPrefix(sel.Sel.Name, "Print") {
				pos := fset.Position(ifs.Pos())
				out = append(out, types.Finding{
					ID:          "ast_weak_err_handler:" + relPath + ":" + strconv.Itoa(pos.Line),
					Tool:        "ast",
					File:        relPath,
					Line:        pos.Line,
					Category:    "error_handling",
					Priority:    "P1",
					FixCategory: "A",
					Message:     "weak error handling: err is only logged, not returned",
				})
			}

			return true
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	return out, nil
}

func isErrCheck(ifs *ast.IfStmt) bool {
	bin, ok := ifs.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op.String() != "!=" {
		return false
	}

	left, lok := bin.X.(*ast.Ident)
	right, rok := bin.Y.(*ast.Ident)
	if !lok || !rok {
		return false
	}

	return left.Name == "err" && right.Name == "nil"
}
