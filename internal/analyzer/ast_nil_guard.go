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

type ASTNilGuardAnalyzer struct{}

func NewASTNilGuardAnalyzer() Analyzer {
	return &ASTNilGuardAnalyzer{}
}

func (a *ASTNilGuardAnalyzer) Name() string {
	return "ast_nil_guard"
}

func (a *ASTNilGuardAnalyzer) Run(projectRoot string) ([]types.Finding, error) {
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

			ptrParams := pointerParamNames(fn)
			if len(ptrParams) == 0 {
				continue
			}

			fnLine := fset.Position(fn.Pos()).Line

			for _, paramName := range ptrParams {
				if hasNilCheckForParam(fn.Body, paramName) {
					continue
				}

				if !hasSelectorUseOfParam(fn.Body, paramName) {
					continue
				}

				out = append(out, types.Finding{
					ID:          "ast_nil_guard:" + relPath + ":" + strconv.Itoa(fnLine) + ":" + paramName,
					Tool:        "ast",
					File:        relPath,
					Line:        fnLine,
					Category:    "nil_safety",
					Priority:    "P2",
					FixCategory: "B",
					Message:     "pointer parameter is used without nil guard",
					Symbol:      paramName,
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

func pointerParamNames(fn *ast.FuncDecl) []string {
	if fn.Type.Params == nil {
		return nil
	}

	var out []string
	for _, field := range fn.Type.Params.List {
		_, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}

		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}

	return out
}

func hasNilCheckForParam(body *ast.BlockStmt, paramName string) bool {
	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}

		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || bin.Op.String() != "==" {
			return true
		}

		left, lok := bin.X.(*ast.Ident)
		right, rok := bin.Y.(*ast.Ident)
		if !lok || !rok {
			return true
		}

		if left.Name == paramName && right.Name == "nil" {
			found = true
			return false
		}
		if right.Name == paramName && left.Name == "nil" {
			found = true
			return false
		}

		return true
	})

	return found
}

func hasSelectorUseOfParam(body *ast.BlockStmt, paramName string) bool {
	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}

		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		if x.Name != paramName {
			return true
		}

		found = true
		return false
	})

	return found
}
