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

type DeferCloseTemplateFixer struct{}

func NewDeferCloseTemplateFixer() Fixer {
	return &DeferCloseTemplateFixer{}
}

func (f *DeferCloseTemplateFixer) Name() string {
	return "defer_close_template"
}

func (f *DeferCloseTemplateFixer) CanApply(find types.Finding) bool {
	return find.Category == "resource_management" && find.FixCategory == "A"
}

func (f *DeferCloseTemplateFixer) Apply(projectRoot string, find types.Finding, functions []types.FunctionInfo) (types.FixResult, error) {
	_ = functions

	result := types.FixResult{
		FindingID:   find.ID,
		File:        find.File,
		Line:        find.Line,
		Strategy:    "template",
		Description: "insert defer Close() after successful os.Open call",
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

		if !lineInsideFunc(fset, fn, find.Line) {
			continue
		}

		if hasAnyDeferClose(fn) {
			result.Error = "defer close already exists in function"
			return result, nil
		}

		for i := 0; i < len(fn.Body.List); i++ {
			assign, ok := fn.Body.List[i].(*ast.AssignStmt)
			if !ok || len(assign.Lhs) < 2 || len(assign.Rhs) != 1 {
				continue
			}

			pos := fset.Position(assign.Pos())
			if pos.Line != find.Line {
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

			fileVar, ok := assign.Lhs[0].(*ast.Ident)
			if !ok || fileVar.Name == "_" {
				continue
			}

			deferStmt := &ast.DeferStmt{
				Call: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(fileVar.Name),
						Sel: ast.NewIdent("Close"),
					},
				},
			}

			insertAt := i + 1
			if insertAt < len(fn.Body.List) {
				if ifs, ok := fn.Body.List[insertAt].(*ast.IfStmt); ok {
					if isErrCheck(ifs) {
						insertAt++
					}
				}
			}

			fn.Body.List = insertStmt(fn.Body.List, insertAt, deferStmt)
			changed = true
			break
		}

		if changed {
			break
		}
	}

	if !changed {
		result.Error = "no safe os.Open pattern found"
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

func lineInsideFunc(fset *token.FileSet, fn *ast.FuncDecl, line int) bool {
	start := fset.Position(fn.Pos()).Line
	end := fset.Position(fn.End()).Line
	return line >= start && line <= end
}

func hasAnyDeferClose(fn *ast.FuncDecl) bool {
	found := false

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		deferStmt, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}

		call, ok := deferStmt.Call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if call.Sel.Name == "Close" {
			found = true
			return false
		}

		return true
	})

	return found
}

func insertStmt(stmts []ast.Stmt, idx int, stmt ast.Stmt) []ast.Stmt {
	if idx < 0 || idx > len(stmts) {
		return stmts
	}

	stmts = append(stmts, nil)
	copy(stmts[idx+1:], stmts[idx:])
	stmts[idx] = stmt
	return stmts
}
