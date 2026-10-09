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
	"strconv"

	"ai_agent/internal/types"
)

type NilGuardTemplateFixer struct{}

func NewNilGuardTemplateFixer() Fixer {
	return &NilGuardTemplateFixer{}
}

func (f *NilGuardTemplateFixer) Name() string {
	return "nil_guard_template"
}

func (f *NilGuardTemplateFixer) CanApply(find types.Finding) bool {
	return find.Category == "nil_safety" && find.FixCategory == "B"
}

func (f *NilGuardTemplateFixer) Apply(projectRoot string, find types.Finding, functions []types.FunctionInfo) (types.FixResult, error) {
	result := types.FixResult{
		FindingID:   find.ID,
		File:        find.File,
		Line:        find.Line,
		Strategy:    "template",
		Description: "insert nil guard for pointer parameter at function entry",
	}

	paramName := find.Symbol
	if paramName == "" {
		result.Error = "nil-guard finding has no parameter symbol"
		return result, nil
	}

	fnInfo, ok := findNilGuardOwner(find, functions)
	if !ok {
		result.Error = "could not determine owning function for nil-guard"
		return result, nil
	}

	if !fnInfo.ReturnsErr {
		result.Error = "owning function does not return error"
		return result, nil
	}

	if !supportsErrorReturnTemplate(fnInfo) {
		result.Error = "function signature is unsupported for safe nil-guard return"
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

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		if fn.Name == nil || fn.Name.Name != fnInfo.Name {
			continue
		}

		if !functionHasPointerParam(fn, paramName) {
			continue
		}

		if hasNilCheckForParamFix(fn.Body, paramName) {
			result.Error = "nil guard already exists in function"
			return result, nil
		}

		ensureSimpleImport(file, "fmt")

		nilGuard := &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  ast.NewIdent(paramName),
				Op: token.EQL,
				Y:  ast.NewIdent("nil"),
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ReturnStmt{
						Results: buildNilGuardReturnResults(fnInfo, paramName),
					},
				},
			},
		}

		fn.Body.List = insertStmt(fn.Body.List, 0, nilGuard)
		changed = true
		break
	}

	if !changed {
		result.Error = "no safe nil-guard insertion point found"
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

func findNilGuardOwner(find types.Finding, functions []types.FunctionInfo) (types.FunctionInfo, bool) {
	paramName := find.Symbol
	bestIdx := -1
	bestDistance := 1 << 30

	for i, fn := range functions {
		if fn.File != find.File {
			continue
		}
		if !fn.ReturnsErr {
			continue
		}
		if !functionInfoHasParam(fn, paramName) {
			continue
		}

		distance := fn.Line - find.Line
		if distance < 0 {
			distance = -distance
		}

		if bestIdx == -1 || distance < bestDistance {
			bestIdx = i
			bestDistance = distance
		}
	}

	if bestIdx == -1 {
		return types.FunctionInfo{}, false
	}

	return functions[bestIdx], true
}

func functionInfoHasParam(fn types.FunctionInfo, paramName string) bool {
	for _, p := range fn.Params {
		if p.Name == paramName {
			return true
		}
	}
	return false
}

func functionHasPointerParam(fn *ast.FuncDecl, paramName string) bool {
	if fn.Type.Params == nil {
		return false
	}

	for _, field := range fn.Type.Params.List {
		_, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}

		for _, name := range field.Names {
			if name.Name == paramName {
				return true
			}
		}
	}

	return false
}

func hasNilCheckForParamFix(body *ast.BlockStmt, paramName string) bool {
	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}

		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || bin.Op != token.EQL {
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

func buildNilGuardReturnResults(fn types.FunctionInfo, paramName string) []ast.Expr {
	var out []ast.Expr

	for _, r := range fn.Results {
		if r == "error" {
			out = append(out, &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("fmt"),
					Sel: ast.NewIdent("Errorf"),
				},
				Args: []ast.Expr{
					&ast.BasicLit{
						Kind:  token.STRING,
						Value: strconv.Quote(paramName + " is nil"),
					},
				},
			})
			continue
		}

		out = append(out, zeroValueExpr(r))
	}

	return out
}

func ensureSimpleImport(file *ast.File, pkg string) {
	for _, imp := range file.Imports {
		if imp.Path != nil && imp.Path.Value == strconv.Quote(pkg) {
			return
		}
	}

	newImport := &ast.GenDecl{
		Tok: token.IMPORT,
		Specs: []ast.Spec{
			&ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(pkg),
				},
			},
		},
	}

	insertAt := 0
	for i, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			insertAt = i + 1
			continue
		}
		if insertAt == 0 {
			insertAt = i
		}
		break
	}

	file.Decls = insertDecl(file.Decls, insertAt, newImport)
}

func insertDecl(decls []ast.Decl, idx int, decl ast.Decl) []ast.Decl {
	if idx < 0 || idx > len(decls) {
		return decls
	}

	decls = append(decls, nil)
	copy(decls[idx+1:], decls[idx:])
	decls[idx] = decl
	return decls
}
