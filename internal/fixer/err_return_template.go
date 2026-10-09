package fixer

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"

	"ai_agent/internal/types"
)

type ErrReturnTemplateFixer struct{}

func NewErrReturnTemplateFixer() Fixer {
	return &ErrReturnTemplateFixer{}
}

func (f *ErrReturnTemplateFixer) Name() string {
	return "err_return_template"
}

func (f *ErrReturnTemplateFixer) CanApply(find types.Finding) bool {
	return find.Category == "error_handling" && find.FixCategory == "A"
}

func (f *ErrReturnTemplateFixer) Apply(projectRoot string, find types.Finding, functions []types.FunctionInfo) (types.FixResult, error) {
	result := types.FixResult{
		FindingID:   find.ID,
		File:        find.File,
		Line:        find.Line,
		Strategy:    "template",
		Description: "replace weak error-only logging with immediate return using zero values + err",
	}

	fnInfo, ok := findOwningFunction(find, functions)
	if !ok || !fnInfo.ReturnsErr {
		result.Error = "owning function does not return error"
		return result, nil
	}

	if !supportsErrorReturnTemplate(fnInfo) {
		result.Error = "function signature is unsupported for safe template return"
		return result, nil
	}

	fullPath := find.File
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(projectRoot, find.File)
	}
	result.File = fullPath

	original, err := os.ReadFile(fullPath)
	if err != nil {
		return result, fmt.Errorf("read file: %w", err)
	}

	backupPath := fullPath + ".bak"
	if err := os.WriteFile(backupPath, original, 0o644); err != nil {
		return result, fmt.Errorf("write backup: %w", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, fullPath, original, parser.ParseComments)
	if err != nil {
		return result, fmt.Errorf("parse file: %w", err)
	}

	changed := false

	ast.Inspect(file, func(n ast.Node) bool {
		if changed {
			return false
		}

		ifs, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}

		pos := fset.Position(ifs.Pos())
		if pos.Line != find.Line {
			return true
		}

		if !isErrCheck(ifs) {
			return true
		}

		ifs.Body.List = []ast.Stmt{
			&ast.ReturnStmt{
				Results: buildReturnResults(fnInfo),
			},
		}
		changed = true
		return false
	})

	if !changed {
		result.Error = "no safe template replacement found"
		return result, nil
	}

	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return result, fmt.Errorf("print AST: %w", err)
	}

	if err := os.WriteFile(fullPath, buf.Bytes(), 0o644); err != nil {
		return result, fmt.Errorf("write file: %w", err)
	}

	result.Applied = true
	return result, nil
}

func supportsErrorReturnTemplate(fn types.FunctionInfo) bool {
	if len(fn.Results) == 0 {
		return false
	}

	for _, r := range fn.Results {
		switch {
		case r == "error":
		case r == "string":
		case r == "int":
		case r == "bool":
		case r == "[]byte":
		case hasPrefix(r, "*"):
		default:
			return false
		}
	}

	return true
}

func buildReturnResults(fn types.FunctionInfo) []ast.Expr {
	var out []ast.Expr

	for _, r := range fn.Results {
		if r == "error" {
			out = append(out, ast.NewIdent("err"))
			continue
		}
		out = append(out, zeroValueExpr(r))
	}

	return out
}

func zeroValueExpr(t string) ast.Expr {
	switch {
	case t == "string":
		return &ast.BasicLit{Kind: token.STRING, Value: `""`}
	case t == "int":
		return &ast.BasicLit{Kind: token.INT, Value: "0"}
	case t == "bool":
		return ast.NewIdent("false")
	case t == "[]byte":
		return ast.NewIdent("nil")
	case hasPrefix(t, "*"):
		return ast.NewIdent("nil")
	default:
		return ast.NewIdent("nil")
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func findOwningFunction(find types.Finding, functions []types.FunctionInfo) (types.FunctionInfo, bool) {
	var best types.FunctionInfo
	found := false

	for _, fn := range functions {
		if fn.File != find.File {
			continue
		}
		if fn.Line <= find.Line {
			if !found || fn.Line > best.Line {
				best = fn
				found = true
			}
		}
	}

	return best, found
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
