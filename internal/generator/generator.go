package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"ai_agent/internal/types"
)

type targetRef struct {
	File string
	Func string
}

type resolvedTarget struct {
	target targetRef
	fn     types.FunctionInfo
}

type fileTemplateData struct {
	PackageName string
	Imports     []string
	Blocks      []string
}

type functionTemplateData struct {
	TestName    string
	StructLines string
	CaseLines   string
	AssertBlock string
}

func GenerateTests(projectRoot string, spec types.Spec, functions []types.FunctionInfo) []types.TestGenerationResult {
	targets := parseTargets(spec.Targets)
	results := make([]types.TestGenerationResult, 0, len(targets))

	grouped := make(map[string][]resolvedTarget)
	indexByTarget := make(map[string]int)

	for _, t := range targets {
		res := types.TestGenerationResult{
			Target:      normalizeTargetKey(t.File, t.Func),
			Description: "generate or update ai-agent tests for target function",
		}

		fn, ok := findFunction(projectRoot, functions, t)
		if !ok {
			res.Error = "target function not found"
			results = append(results, res)
			continue
		}

		if fn.Receiver != "" {
			res.Error = "methods are skipped in current MVP generator"
			results = append(results, res)
			continue
		}

		if !isSupportedSignature(fn) {
			res.Error = "unsupported function signature for MVP generator"
			results = append(results, res)
			continue
		}

		testFilePath := buildTestFilePath(projectRoot, fn.File)
		res.File = testFilePath

		results = append(results, res)
		idx := len(results) - 1
		indexByTarget[res.Target] = idx

		grouped[testFilePath] = append(grouped[testFilePath], resolvedTarget{
			target: t,
			fn:     fn,
		})
	}

	filePaths := make([]string, 0, len(grouped))
	for path := range grouped {
		filePaths = append(filePaths, path)
	}
	sort.Strings(filePaths)

	for _, testFilePath := range filePaths {
		group := grouped[testFilePath]

		content, err := renderCombinedTestFile(spec, group)
		if err != nil {
			for _, item := range group {
				key := normalizeTargetKey(item.target.File, item.target.Func)
				if idx, ok := indexByTarget[key]; ok {
					results[idx].Error = err.Error()
					results[idx].Generated = false
				}
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(testFilePath), 0o755); err != nil {
			for _, item := range group {
				key := normalizeTargetKey(item.target.File, item.target.Func)
				if idx, ok := indexByTarget[key]; ok {
					results[idx].Error = "create test directory: " + err.Error()
					results[idx].Generated = false
				}
			}
			continue
		}

		if err := os.WriteFile(testFilePath, []byte(content), 0o644); err != nil {
			for _, item := range group {
				key := normalizeTargetKey(item.target.File, item.target.Func)
				if idx, ok := indexByTarget[key]; ok {
					results[idx].Error = "write generated test file: " + err.Error()
					results[idx].Generated = false
				}
			}
			continue
		}

		info, statErr := os.Stat(testFilePath)
		if statErr != nil {
			for _, item := range group {
				key := normalizeTargetKey(item.target.File, item.target.Func)
				if idx, ok := indexByTarget[key]; ok {
					results[idx].Error = "generated test file not found after write: " + statErr.Error()
					results[idx].Generated = false
				}
			}
			continue
		}
		if info.IsDir() {
			for _, item := range group {
				key := normalizeTargetKey(item.target.File, item.target.Func)
				if idx, ok := indexByTarget[key]; ok {
					results[idx].Error = "generated test path is a directory, not a file"
					results[idx].Generated = false
				}
			}
			continue
		}

		for _, item := range group {
			key := normalizeTargetKey(item.target.File, item.target.Func)
			if idx, ok := indexByTarget[key]; ok {
				results[idx].Generated = true
				results[idx].Error = ""
				results[idx].File = testFilePath
			}
		}
	}

	return results
}

func PlanTests(projectRoot string, spec types.Spec, functions []types.FunctionInfo) []types.TestGenerationResult {
	targets := parseTargets(spec.Targets)
	results := make([]types.TestGenerationResult, 0, len(targets))

	for _, t := range targets {
		res := types.TestGenerationResult{
			Target:      normalizeTargetKey(t.File, t.Func),
			Planned:     true,
			Description: "would generate or update ai-agent tests for target function",
		}

		fn, ok := findFunction(projectRoot, functions, t)
		if !ok {
			res.Error = "target function not found"
			results = append(results, res)
			continue
		}

		if fn.Receiver != "" {
			res.Error = "methods are skipped in current MVP generator"
			results = append(results, res)
			continue
		}

		if !isSupportedSignature(fn) {
			res.Error = "unsupported function signature for MVP generator"
			results = append(results, res)
			continue
		}

		res.File = buildTestFilePath(projectRoot, fn.File)
		results = append(results, res)
	}

	return results
}

func parseTargets(targets []string) []targetRef {
	var out []targetRef

	for _, t := range targets {
		parts := strings.Split(t, ":")
		if len(parts) != 2 {
			continue
		}

		out = append(out, targetRef{
			File: normalizeSlashes(filepath.Clean(parts[0])),
			Func: parts[1],
		})
	}

	return out
}

func findFunction(projectRoot string, functions []types.FunctionInfo, target targetRef) (types.FunctionInfo, bool) {
	targetPath := canonicalPath(projectRoot, target.File)

	for _, fn := range functions {
		fnPath := canonicalPath(projectRoot, fn.File)
		if fnPath == targetPath && fn.Name == target.Func {
			return fn, true
		}
	}

	return types.FunctionInfo{}, false
}

func canonicalPath(projectRoot, path string) string {
	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, p)
	}

	abs, err := filepath.Abs(p)
	if err == nil {
		p = abs
	}

	p = filepath.Clean(p)
	return normalizeSlashes(p)
}

func buildTestFilePath(projectRoot, sourceFile string) string {
	sourcePath := canonicalPath(projectRoot, sourceFile)
	sourcePath = filepath.FromSlash(sourcePath)

	base := filepath.Base(sourcePath)
	name := strings.TrimSuffix(base, ".go") + "_ai_agent_test.go"

	return filepath.Join(filepath.Dir(sourcePath), name)
}

func normalizeTargetKey(file, fn string) string {
	return normalizeSlashes(filepath.Clean(file)) + ":" + fn
}

func normalizeSlashes(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func isSupportedSignature(fn types.FunctionInfo) bool {
	if isPathStringResultError(fn) {
		return true
	}
	if isPathErrorOnly(fn) {
		return true
	}

	if len(fn.Params) > 2 {
		return false
	}

	for _, p := range fn.Params {
		if !isSupportedType(p.Type) {
			return false
		}
	}

	for _, r := range fn.Results {
		if !isSupportedResultType(r) {
			return false
		}
	}

	return true
}

func isSupportedType(t string) bool {
	switch {
	case t == "string", t == "int", t == "bool", t == "[]byte":
		return true
	case strings.HasPrefix(t, "*"):
		return true
	default:
		return false
	}
}

func isSupportedResultType(t string) bool {
	switch t {
	case "bool", "string", "int", "error":
		return true
	default:
		return false
	}
}

func renderCombinedTestFile(spec types.Spec, group []resolvedTarget) (string, error) {
	if len(group) == 0 {
		return "", fmt.Errorf("empty target group")
	}

	packageName := group[0].fn.Package
	imports := buildImports(spec, group)

	var blocks []string
	for _, item := range group {
		block, err := renderTestFunction(spec, item.fn)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, block)
	}

	data := fileTemplateData{
		PackageName: packageName,
		Imports:     imports,
		Blocks:      blocks,
	}

	tpl := `package {{.PackageName}}

import (
{{- range .Imports }}
	{{ printf "%q" . }}
{{- end }}
)

{{- range .Blocks }}

{{ . }}
{{- end }}
`

	var buf bytes.Buffer
	t, err := template.New("file").Parse(tpl)
	if err != nil {
		return "", err
	}

	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderTestFunction(spec types.Spec, fn types.FunctionInfo) (string, error) {
	if isPathStringResultError(fn) {
		return renderPathReadFunctionTest(fn)
	}
	if isPathErrorOnly(fn) {
		return renderPathErrorOnlyFunctionTest(fn)
	}

	data := functionTemplateData{
		TestName:    "Test_" + fn.Name + "_AiAgent",
		StructLines: buildStructLines(fn),
		CaseLines:   buildCaseLines(spec, fn),
		AssertBlock: buildAssertBlock(fn),
	}

	tpl := `func {{.TestName}}(t *testing.T) {
	tests := []struct {
		name string
{{.StructLines}}	}{
{{.CaseLines}}	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
{{.AssertBlock}}		})
	}
}`

	var buf bytes.Buffer
	t, err := template.New("function").Parse(tpl)
	if err != nil {
		return "", err
	}

	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderPathReadFunctionTest(fn types.FunctionInfo) (string, error) {
	testName := "Test_" + fn.Name + "_AiAgent"

	tpl := `func ` + testName + `(t *testing.T) {
	t.Run("missing_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")

		_, err := ` + fn.Name + `(path)
		if err == nil {
			t.Fatalf("expected error for missing file")
		}
	})

	t.Run("single_byte_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sample.txt")

		if err := os.WriteFile(path, []byte("A"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		got, err := ` + fn.Name + `(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "A" {
			t.Fatalf("got=%v want=%v", got, "A")
		}
	})
}`

	return tpl, nil
}

func renderPathErrorOnlyFunctionTest(fn types.FunctionInfo) (string, error) {
	testName := "Test_" + fn.Name + "_AiAgent"

	tpl := `func ` + testName + `(t *testing.T) {
	t.Run("missing_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.txt")

		err := ` + fn.Name + `(path)
		if err == nil {
			t.Fatalf("expected error for missing file")
		}
	})

	t.Run("existing_file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sample.txt")

		if err := os.WriteFile(path, []byte("A"), 0o644); err != nil {
			t.Fatalf("write temp file: %v", err)
		}

		err := ` + fn.Name + `(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("expected file to be removed, stat err=%v", statErr)
		}
	})
}`

	return tpl, nil
}

func buildImports(spec types.Spec, group []resolvedTarget) []string {
	set := map[string]bool{
		"testing": true,
	}

	for _, s := range spec.TestScenarios {
		ls := strings.ToLower(s)
		if strings.Contains(ls, "large") || strings.Contains(ls, "10kb") {
			set["strings"] = true
		}
	}

	for _, item := range group {
		if isPathStringResultError(item.fn) || isPathErrorOnly(item.fn) {
			set["os"] = true
			set["path/filepath"] = true
		}
	}

	var imports []string
	for k := range set {
		imports = append(imports, k)
	}
	sort.Strings(imports)

	return imports
}

func buildStructLines(fn types.FunctionInfo) string {
	var b strings.Builder

	for idx, p := range fn.Params {
		fieldName := p.Name
		if fieldName == "" {
			fieldName = fmt.Sprintf("arg%d", idx)
		}
		b.WriteString(fmt.Sprintf("\t\t%s %s\n", fieldName, p.Type))
	}

	for i, r := range fn.Results {
		if r == "error" {
			b.WriteString("\t\twantErr bool\n")
		} else {
			b.WriteString(fmt.Sprintf("\t\twant%d %s\n", i, r))
		}
	}

	return b.String()
}

func buildCaseLines(spec types.Spec, fn types.FunctionInfo) string {
	var lines []string
	lines = append(lines, buildScenarioNamedCase("default_case", "default", fn))

	if hasPointerParam(fn) {
		for _, s := range spec.TestScenarios {
			ls := strings.ToLower(s)
			if strings.Contains(ls, "nil") {
				lines = append(lines, buildScenarioNamedCase("nil_dependencies", "nil", fn))
			}
		}
		return strings.Join(unique(lines), "\n")
	}

	for _, s := range spec.TestScenarios {
		ls := strings.ToLower(s)

		switch {
		case strings.Contains(ls, "empty"):
			lines = append(lines, buildScenarioNamedCase("empty_input", "empty", fn))
		case strings.Contains(ls, "invalid"):
			lines = append(lines, buildScenarioNamedCase("invalid_input", "invalid", fn))
		case strings.Contains(ls, "large") || strings.Contains(ls, "10kb"):
			lines = append(lines, buildScenarioNamedCase("large_input", "large", fn))
		}
	}

	return strings.Join(unique(lines), "\n")
}

func buildScenarioNamedCase(name, mode string, fn types.FunctionInfo) string {
	args := buildCaseArgs(fn, mode)
	results := buildCaseResults(fn, mode, hasPointerParam(fn))
	return fmt.Sprintf("\t\t{name: %q%s%s},", name, args, results)
}

func buildCaseArgs(fn types.FunctionInfo, mode string) string {
	var parts []string

	for idx, p := range fn.Params {
		fieldName := p.Name
		if fieldName == "" {
			fieldName = fmt.Sprintf("arg%d", idx)
		}
		parts = append(parts, fmt.Sprintf(", %s: %s", fieldName, sampleValueForType(p.Type, mode)))
	}

	return strings.Join(parts, "")
}

func buildCaseResults(fn types.FunctionInfo, mode string, pointerMode bool) string {
	var parts []string

	for i, r := range fn.Results {
		if r == "error" {
			if pointerMode {
				if mode == "nil" {
					parts = append(parts, ", wantErr: true")
				} else {
					parts = append(parts, ", wantErr: false")
				}
				continue
			}

			switch mode {
			case "empty", "invalid", "nil":
				parts = append(parts, ", wantErr: true")
			default:
				parts = append(parts, ", wantErr: false")
			}
			continue
		}

		switch r {
		case "bool":
			if mode == "empty" || mode == "invalid" || mode == "nil" {
				parts = append(parts, fmt.Sprintf(", want%d: false", i))
			} else {
				parts = append(parts, fmt.Sprintf(", want%d: true", i))
			}
		case "string":
			parts = append(parts, fmt.Sprintf(", want%d: %q", i, expectedStringValue(fn, mode)))
		case "int":
			parts = append(parts, fmt.Sprintf(", want%d: %d", i, 0))
		}
	}

	return strings.Join(parts, "")
}

func expectedStringValue(fn types.FunctionInfo, mode string) string {
	if fn.Name == "UserLabel" && mode == "default" {
		return "User: Alice"
	}
	return ""
}

func sampleValueForType(t, mode string) string {
	switch {
	case t == "string":
		switch mode {
		case "empty":
			return `""`
		case "large":
			return `strings.Repeat("a", 11*1024) + "@example.com"`
		case "invalid":
			return `"invalid-email-without-at-sign"`
		default:
			return `"sample@example.com"`
		}

	case t == "int":
		if mode == "empty" {
			return "0"
		}
		return "1"

	case t == "bool":
		if mode == "invalid" {
			return "false"
		}
		return "true"

	case t == "[]byte":
		switch mode {
		case "empty":
			return "[]byte{}"
		case "large":
			return `[]byte(strings.Repeat("a", 11*1024))`
		default:
			return `[]byte("sample")`
		}

	case strings.HasPrefix(t, "*"):
		if mode == "nil" {
			return "nil"
		}
		typeName := strings.TrimPrefix(t, "*")
		if typeName == "User" {
			return `&User{Name: "Alice"}`
		}
		if strings.Contains(typeName, ".") {
			return "nil"
		}
		return "&" + typeName + "{}"

	default:
		return "nil"
	}
}

func buildCallArgs(fn types.FunctionInfo) string {
	var args []string

	for idx, p := range fn.Params {
		fieldName := p.Name
		if fieldName == "" {
			fieldName = fmt.Sprintf("arg%d", idx)
		}
		args = append(args, "tt."+fieldName)
	}

	return strings.Join(args, ", ")
}

func buildAssertBlock(fn types.FunctionInfo) string {
	hasErr := false
	nonErrCount := 0

	for _, r := range fn.Results {
		if r == "error" {
			hasErr = true
		} else {
			nonErrCount++
		}
	}

	if hasPointerParam(fn) && hasErr && nonErrCount == 1 {
		return fmt.Sprintf(`			got, err := %s(%s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got err=%%v, wantErr=%%v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want0 {
				t.Fatalf("got=%%v want=%%v", got, tt.want0)
			}
`, fn.Name, buildCallArgs(fn))
	}

	switch {
	case len(fn.Results) == 0:
		return fmt.Sprintf(`			%s(%s)
`, fn.Name, buildCallArgs(fn))

	case len(fn.Results) == 1 && fn.Results[0] == "error":
		return fmt.Sprintf(`			err := %s(%s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got err=%%v, wantErr=%%v", err, tt.wantErr)
			}
`, fn.Name, buildCallArgs(fn))

	case len(fn.Results) == 1:
		return fmt.Sprintf(`			got := %s(%s)
			if got != tt.want0 {
				t.Fatalf("got=%%v want=%%v", got, tt.want0)
			}
`, fn.Name, buildCallArgs(fn))

	case hasErr && nonErrCount == 1:
		return fmt.Sprintf(`			got, err := %s(%s)
			if (err != nil) != tt.wantErr {
				t.Fatalf("got err=%%v, wantErr=%%v", err, tt.wantErr)
			}
			if got != tt.want0 {
				t.Fatalf("got=%%v want=%%v", got, tt.want0)
			}
`, fn.Name, buildCallArgs(fn))

	default:
		return fmt.Sprintf(`			// unsupported assertion pattern for %s
`, fn.Name)
	}
}

func hasPointerParam(fn types.FunctionInfo) bool {
	for _, p := range fn.Params {
		if strings.HasPrefix(p.Type, "*") {
			return true
		}
	}
	return false
}

func isPathStringResultError(fn types.FunctionInfo) bool {
	if len(fn.Params) != 1 {
		return false
	}
	if fn.Params[0].Type != "string" {
		return false
	}

	paramName := strings.ToLower(fn.Params[0].Name)
	if !strings.Contains(paramName, "path") && !strings.Contains(paramName, "file") {
		return false
	}

	if len(fn.Results) != 2 {
		return false
	}

	return fn.Results[0] == "string" && fn.Results[1] == "error"
}

func isPathErrorOnly(fn types.FunctionInfo) bool {
	if len(fn.Params) != 1 {
		return false
	}
	if fn.Params[0].Type != "string" {
		return false
	}

	paramName := strings.ToLower(fn.Params[0].Name)
	if !strings.Contains(paramName, "path") && !strings.Contains(paramName, "file") {
		return false
	}

	return len(fn.Results) == 1 && fn.Results[0] == "error"
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string

	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}

	return out
}
