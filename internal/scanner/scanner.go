package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"

	"ai_agent/internal/types"
)

type ScanResult struct {
	Files     []string
	Functions []types.FunctionInfo
}

func ScanProject(root string) (*ScanResult, error) {
	result := &ScanResult{}
	fset := token.NewFileSet()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relPath := toProjectRelative(absRoot, path)
		result.Files = append(result.Files, relPath)

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		pkg := file.Name.Name
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			line := fset.Position(fn.Pos()).Line
			receiver := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				switch t := fn.Recv.List[0].Type.(type) {
				case *ast.Ident:
					receiver = t.Name
				case *ast.StarExpr:
					if ident, ok := t.X.(*ast.Ident); ok {
						receiver = "*" + ident.Name
					}
				}
			}

			returnsErr, results := extractResults(fn)
			params := extractParams(fn)

			result.Functions = append(result.Functions, types.FunctionInfo{
				File:       relPath,
				Package:    pkg,
				Name:       fn.Name.Name,
				Receiver:   receiver,
				Line:       line,
				ReturnsErr: returnsErr,
				Results:    results,
				Params:     params,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func toProjectRelative(projectRoot, path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}

	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		return filepath.Clean(absPath)
	}

	return filepath.Clean(rel)
}

func extractResults(fn *ast.FuncDecl) (bool, []string) {
	if fn.Type.Results == nil {
		return false, nil
	}

	var results []string
	returnsErr := false

	for _, field := range fn.Type.Results.List {
		switch t := field.Type.(type) {
		case *ast.Ident:
			results = append(results, t.Name)
			if t.Name == "error" {
				returnsErr = true
			}
		case *ast.SelectorExpr:
			if x, ok := t.X.(*ast.Ident); ok {
				results = append(results, x.Name+"."+t.Sel.Name)
			} else {
				results = append(results, "unknown")
			}
		default:
			results = append(results, "unknown")
		}
	}

	return returnsErr, results
}

func extractParams(fn *ast.FuncDecl) []types.ParamInfo {
	if fn.Type.Params == nil {
		return nil
	}

	var params []types.ParamInfo
	for _, field := range fn.Type.Params.List {
		typeName := exprToTypeString(field.Type)

		if len(field.Names) == 0 {
			params = append(params, types.ParamInfo{
				Name: "",
				Type: typeName,
			})
			continue
		}

		for _, name := range field.Names {
			params = append(params, types.ParamInfo{
				Name: name.Name,
				Type: typeName,
			})
		}
	}

	return params
}

func exprToTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToTypeString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToTypeString(t.Elt)
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
		return "unknown"
	default:
		return "unknown"
	}
}
