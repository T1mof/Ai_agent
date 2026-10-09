package fixer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai_agent/internal/llm"
	"ai_agent/internal/types"
)

type LLMPatchFixer struct {
	client  llm.Client
	initErr error
}

func NewLLMPatchFixer(client llm.Client, initErr error) *LLMPatchFixer {
	return &LLMPatchFixer{
		client:  client,
		initErr: initErr,
	}
}

func (f *LLMPatchFixer) Name() string {
	return "llm_patch"
}

func (f *LLMPatchFixer) CanApply(fd types.Finding) bool {
	if f.initErr != nil {
		return true
	}
	return f.client != nil
}

func (f *LLMPatchFixer) Apply(
	projectRoot string,
	fd types.Finding,
	functions []types.FunctionInfo,
) (types.FixResult, error) {
	res := types.FixResult{
		FindingID:   fd.ID,
		File:        filepath.Join(projectRoot, fd.File),
		Line:        fd.Line,
		Strategy:    "llm",
		Description: "generate patch via llm and validate through normal pipeline",
	}

	if f.initErr != nil {
		return res, f.initErr
	}
	if f.client == nil {
		return res, fmt.Errorf("llm fixer is disabled")
	}

	filePath := res.File
	src, err := os.ReadFile(filePath)
	if err != nil {
		return res, fmt.Errorf("read file: %w", err)
	}

	extracted, err := extractTargetFunction(filePath, src, fd)
	if err != nil {
		return res, err
	}

	req := llm.PatchRequest{
		Finding:           fd,
		FilePath:          filePath,
		FunctionName:      extracted.FunctionName,
		PackageName:       extracted.PackageName,
		FunctionSource:    extracted.FunctionSource,
		SurroundingSource: extracted.SurroundingSource,
	}

	patch, err := f.generatePatchWithRetry(req)
	if err != nil {
		return res, err
	}

	patch.UpdatedFunction = normalizePatchedFunction(
		patch.UpdatedFunction,
		extracted.FunctionName,
	)

	writeLLMDebugArtifacts(projectRoot, extracted, patch)

	if err := validatePatchedFunction(extracted, patch.UpdatedFunction); err != nil {
		writeLLMValidationFailureArtifacts(
			projectRoot,
			extracted,
			patch.UpdatedFunction,
			err,
		)
		return res, fmt.Errorf("llm produced invalid function: %w", err)
	}

	backupPath := filePath + ".bak"
	if err := os.WriteFile(backupPath, src, 0o644); err != nil {
		return res, fmt.Errorf("create backup: %w", err)
	}

	newFile := string(src[:extracted.StartOffset]) +
		patch.UpdatedFunction +
		string(src[extracted.EndOffset:])

	if formatted, err := format.Source([]byte(newFile)); err == nil {
		newFile = string(formatted)
	}

	if err := os.WriteFile(filePath, []byte(newFile), 0o644); err != nil {
		return res, fmt.Errorf("write patched file: %w", err)
	}

	clearLLMValidationFailureArtifacts(projectRoot)

	res.Applied = true
	return res, nil
}

func (f *LLMPatchFixer) generatePatchWithRetry(
	req llm.PatchRequest,
) (llm.PatchResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		patch, err := f.client.GeneratePatch(ctx, req)
		cancel()

		if err == nil {
			return patch, nil
		}

		lastErr = err
		if !isRetryableLLMError(err) {
			return llm.PatchResponse{}, err
		}
	}

	return llm.PatchResponse{}, lastErr
}

func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable")
}

type extractedFunction struct {
	PackageName       string
	FunctionName      string
	FunctionSource    string
	SurroundingSource string
	StartOffset       int
	EndOffset         int
}

func extractTargetFunction(
	filePath string,
	src []byte,
	finding types.Finding,
) (extractedFunction, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return extractedFunction{}, fmt.Errorf("parse file: %w", err)
	}

	if strings.HasPrefix(finding.ID, "ast_ignored_error:") {
		if out, ok := extractIgnoredErrorFunction(fset, file, src, finding.Line); ok {
			return out, nil
		}
	}

	if out, ok := extractFunctionByLine(fset, file, src, finding.Line); ok {
		return out, nil
	}

	return extractedFunction{}, fmt.Errorf("target function for llm patch not found")
}

func extractIgnoredErrorFunction(
	fset *token.FileSet,
	file *ast.File,
	src []byte,
	findingLine int,
) (extractedFunction, bool) {
	type candidate struct {
		fn        *ast.FuncDecl
		startLine int
		distance  int
	}

	var candidates []candidate

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		if !funcReturnsError(fn) {
			continue
		}
		if !funcContainsIgnoredError(fn) {
			continue
		}

		start := fset.Position(fn.Pos()).Line
		dist := abs(start - findingLine)

		candidates = append(candidates, candidate{
			fn:        fn,
			startLine: start,
			distance:  dist,
		})
	}

	if len(candidates) == 0 {
		return extractedFunction{}, false
	}

	best := candidates[0]

	for _, c := range candidates[1:] {
		bestAfter := best.startLine >= findingLine
		currentAfter := c.startLine >= findingLine

		switch {
		case !bestAfter && currentAfter:
			best = c
		case bestAfter && currentAfter:
			if c.startLine < best.startLine {
				best = c
			}
		case !bestAfter && !currentAfter:
			if c.distance < best.distance {
				best = c
			}
		}
	}

	return buildExtractedFunction(fset, src, file.Name.Name, best.fn)
}

func extractFunctionByLine(
	fset *token.FileSet,
	file *ast.File,
	src []byte,
	line int,
) (extractedFunction, bool) {
	var best *ast.FuncDecl
	bestStart := -1

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		start := fset.Position(fn.Pos())
		end := fset.Position(fn.End())
		if start.Line <= line && line <= end.Line {
			if start.Line > bestStart {
				best = fn
				bestStart = start.Line
			}
		}
	}

	if best == nil {
		return extractedFunction{}, false
	}

	out, ok := buildExtractedFunction(fset, src, file.Name.Name, best)
	return out, ok
}

func buildExtractedFunction(
	fset *token.FileSet,
	src []byte,
	packageName string,
	fn *ast.FuncDecl,
) (extractedFunction, bool) {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

	if startPos.Offset < 0 || endPos.Offset < 0 || endPos.Offset > len(src) || startPos.Offset >= endPos.Offset {
		return extractedFunction{}, false
	}

	fnSource := string(src[startPos.Offset:endPos.Offset])

	lines := strings.Split(string(src), "\n")
	from := startPos.Line - 3
	if from < 1 {
		from = 1
	}
	to := endPos.Line + 3
	if to > len(lines) {
		to = len(lines)
	}

	surrounding := strings.Join(lines[from-1:to], "\n")

	return extractedFunction{
		PackageName:       packageName,
		FunctionName:      fn.Name.Name,
		FunctionSource:    fnSource,
		SurroundingSource: surrounding,
		StartOffset:       startPos.Offset,
		EndOffset:         endPos.Offset,
	}, true
}

func funcReturnsError(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "error" {
			return true
		}
	}
	return false
}

func funcContainsIgnoredError(fn *ast.FuncDecl) bool {
	found := false

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}

		id, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || id.Name != "_" {
			return true
		}

		if _, ok := assign.Rhs[0].(*ast.CallExpr); ok {
			found = true
			return false
		}

		return true
	})

	return found
}

func validatePatchedFunction(
	target extractedFunction,
	patchedSource string,
) error {
	originalFn, err := parseSingleFunction(target.PackageName, target.FunctionSource)
	if err != nil {
		return fmt.Errorf("parse original target function: %w", err)
	}

	patchedFn, err := parseSingleFunction(target.PackageName, patchedSource)
	if err != nil {
		return fmt.Errorf("parse patched function: %w", err)
	}

	if patchedFn.Name == nil || patchedFn.Name.Name != target.FunctionName {
		return fmt.Errorf(
			"patched function name mismatch: got %q want %q",
			patchedFn.Name.Name,
			target.FunctionName,
		)
	}

	if originalFn.Name == nil || originalFn.Name.Name != patchedFn.Name.Name {
		return fmt.Errorf("patched function does not match original function name")
	}

	origHeader, err := functionHeaderString(originalFn)
	if err != nil {
		return fmt.Errorf("format original function header: %w", err)
	}

	patchedHeader, err := functionHeaderString(patchedFn)
	if err != nil {
		return fmt.Errorf("format patched function header: %w", err)
	}

	if origHeader != patchedHeader {
		return fmt.Errorf(
			"patched function signature changed:\noriginal: %s\npatched:  %s",
			origHeader,
			patchedHeader,
		)
	}

	if patchedFn.Body == nil || len(patchedFn.Body.List) == 0 {
		return fmt.Errorf("patched function body is empty")
	}

	return nil
}

func parseSingleFunction(pkgName, fnSource string) (*ast.FuncDecl, error) {
	testSource := "package " + pkgName + "\n\n" + fnSource + "\n"
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "patched.go", testSource, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	if len(file.Decls) != 1 {
		return nil, fmt.Errorf("expected exactly one declaration, got %d", len(file.Decls))
	}

	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		return nil, fmt.Errorf("declaration is not a function")
	}

	return fn, nil
}

func functionHeaderString(fn *ast.FuncDecl) (string, error) {
	if fn == nil {
		return "", fmt.Errorf("nil function")
	}

	synthetic := &ast.FuncDecl{
		Recv: fn.Recv,
		Name: ast.NewIdent(fn.Name.Name),
		Type: fn.Type,
		Body: &ast.BlockStmt{},
	}

	fset := token.NewFileSet()
	var buf bytes.Buffer

	if err := format.Node(&buf, fset, synthetic); err != nil {
		return "", err
	}

	header := strings.TrimSpace(buf.String())
	header = strings.TrimSuffix(header, "{}")
	header = strings.TrimSpace(header)

	return header, nil
}

func normalizePatchedFunction(patchedSource, functionName string) string {
	s := strings.TrimSpace(patchedSource)
	s = stripMarkdownFences(s)

	if strings.HasPrefix(s, functionName+"(") {
		s = "func " + s
	}

	if strings.HasPrefix(s, "function "+functionName) {
		s = strings.TrimPrefix(s, "function ")
		if strings.HasPrefix(s, functionName+"(") {
			s = "func " + s
		}
	}

	if !strings.HasPrefix(s, "func ") {
		if idx := strings.Index(s, "func "+functionName+"("); idx >= 0 {
			s = s[idx:]
		} else if idx := strings.Index(s, functionName+"("); idx >= 0 {
			s = "func " + s[idx:]
		}
	}

	return strings.TrimSpace(s)
}

func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```go")
	s = strings.TrimPrefix(s, "```golang")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func writeLLMDebugArtifacts(
	projectRoot string,
	extracted extractedFunction,
	patch llm.PatchResponse,
) {
	_ = os.WriteFile(
		filepath.Join(projectRoot, "llm_target_function.txt"),
		[]byte(extracted.FunctionSource),
		0o644,
	)
	_ = os.WriteFile(
		filepath.Join(projectRoot, "llm_last_patch.txt"),
		[]byte(patch.UpdatedFunction),
		0o644,
	)
	_ = os.WriteFile(
		filepath.Join(projectRoot, "llm_last_rationale.txt"),
		[]byte(patch.Rationale),
		0o644,
	)
}

func writeLLMValidationFailureArtifacts(
	projectRoot string,
	extracted extractedFunction,
	patchedSource string,
	err error,
) {
	_ = os.WriteFile(
		filepath.Join(projectRoot, "llm_original_function.txt"),
		[]byte(extracted.FunctionSource),
		0o644,
	)
	_ = os.WriteFile(
		filepath.Join(projectRoot, "llm_rejected_patch.txt"),
		[]byte(patchedSource),
		0o644,
	)
	_ = os.WriteFile(
		filepath.Join(projectRoot, "llm_validation_error.txt"),
		[]byte(err.Error()),
		0o644,
	)
}

func clearLLMValidationFailureArtifacts(projectRoot string) {
	_ = os.Remove(filepath.Join(projectRoot, "llm_original_function.txt"))
	_ = os.Remove(filepath.Join(projectRoot, "llm_rejected_patch.txt"))
	_ = os.Remove(filepath.Join(projectRoot, "llm_validation_error.txt"))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
