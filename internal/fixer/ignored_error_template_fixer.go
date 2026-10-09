package fixer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai_agent/internal/types"
)

type IgnoredErrorTemplateFixer struct{}

func NewIgnoredErrorTemplateFixer() *IgnoredErrorTemplateFixer {
	return &IgnoredErrorTemplateFixer{}
}

func (f *IgnoredErrorTemplateFixer) Name() string {
	return "ignored_error_template"
}

func (f *IgnoredErrorTemplateFixer) CanApply(fd types.Finding) bool {
	return strings.HasPrefix(fd.ID, "ast_ignored_error:")
}

func (f *IgnoredErrorTemplateFixer) Apply(
	projectRoot string,
	fd types.Finding,
	functions []types.FunctionInfo,
) (types.FixResult, error) {
	res := types.FixResult{
		FindingID:   fd.ID,
		File:        filepath.Join(projectRoot, fd.File),
		Line:        fd.Line,
		Strategy:    "template",
		Description: "replace ignored error assignment with checked call and early return",
	}

	filePath := res.File
	content, err := os.ReadFile(filePath)
	if err != nil {
		return res, fmt.Errorf("read file: %w", err)
	}

	backupPath := filePath + ".bak"
	if err := os.WriteFile(backupPath, content, 0o644); err != nil {
		return res, fmt.Errorf("create backup: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return res, fmt.Errorf("empty file")
	}

	targetIdx := findIgnoredErrorLine(lines, fd.Line)
	if targetIdx < 0 {
		return res, fmt.Errorf("ignored-error assignment line not found")
	}

	fn, ok := findEnclosingFunction(fd, functions)
	if !ok {
		return res, fmt.Errorf("enclosing function not found")
	}
	if !fn.ReturnsErr {
		return res, fmt.Errorf("function does not return error, safe template not available")
	}

	line := lines[targetIdx]
	indent := leadingWhitespace(line)
	trimmed := strings.TrimSpace(line)

	if !strings.HasPrefix(trimmed, "_ = ") {
		return res, fmt.Errorf("unsupported ignored-error pattern")
	}

	callExpr := strings.TrimSpace(strings.TrimPrefix(trimmed, "_ = "))
	callExpr = stripTrailingComment(callExpr)
	if callExpr == "" {
		return res, fmt.Errorf("empty ignored-error expression")
	}

	returnStmt := buildErrorReturn(fn)
	if returnStmt == "" {
		return res, fmt.Errorf("could not build return statement")
	}

	replacement := []string{
		indent + "if err := " + callExpr + "; err != nil {",
		indent + "\t" + returnStmt,
		indent + "}",
	}

	lines = append(lines[:targetIdx], append(replacement, lines[targetIdx+1:]...)...)
	newContent := strings.Join(lines, "\n")

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return res, fmt.Errorf("write file: %w", err)
	}

	res.Applied = true
	return res, nil
}

func findIgnoredErrorLine(lines []string, reportedLine int) int {
	candidates := []int{
		reportedLine - 1,
		reportedLine,
		reportedLine - 2,
		reportedLine + 1,
	}

	for _, idx := range candidates {
		if idx < 0 || idx >= len(lines) {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(lines[idx]), "_ = ") {
			return idx
		}
	}

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "_ = ") {
			return i
		}
	}

	return -1
}

func findEnclosingFunction(fd types.Finding, functions []types.FunctionInfo) (types.FunctionInfo, bool) {
	targetFile := normalizeFixerPath(fd.File)

	bestLine := -1
	var best types.FunctionInfo

	for _, fn := range functions {
		if normalizeFixerPath(fn.File) != targetFile {
			continue
		}
		if fn.Line <= fd.Line && fn.Line > bestLine {
			best = fn
			bestLine = fn.Line
		}
	}

	if bestLine == -1 {
		return types.FunctionInfo{}, false
	}

	return best, true
}

func normalizeFixerPath(p string) string {
	p = filepath.Clean(p)
	p = strings.ReplaceAll(p, "\\", "/")
	return p
}

func leadingWhitespace(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '\t' {
			b.WriteRune(r)
			continue
		}
		break
	}
	return b.String()
}

func stripTrailingComment(s string) string {
	if idx := strings.Index(s, "//"); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func buildErrorReturn(fn types.FunctionInfo) string {
	if len(fn.Results) == 0 {
		return ""
	}

	var parts []string
	for _, r := range fn.Results {
		if r == "error" {
			parts = append(parts, "err")
			continue
		}
		parts = append(parts, zeroValue(r))
	}

	return "return " + strings.Join(parts, ", ")
}

func zeroValue(t string) string {
	switch {
	case t == "string":
		return `""`
	case t == "int", t == "int8", t == "int16", t == "int32", t == "int64":
		return "0"
	case t == "uint", t == "uint8", t == "uint16", t == "uint32", t == "uint64", t == "uintptr":
		return "0"
	case t == "float32", t == "float64":
		return "0"
	case t == "bool":
		return "false"
	case strings.HasPrefix(t, "*"):
		return "nil"
	case strings.HasPrefix(t, "[]"):
		return "nil"
	case strings.HasPrefix(t, "map["):
		return "nil"
	case strings.HasPrefix(t, "chan "):
		return "nil"
	case strings.HasPrefix(t, "func("):
		return "nil"
	case strings.HasPrefix(t, "interface{"):
		return "nil"
	default:
		return "nil"
	}
}
