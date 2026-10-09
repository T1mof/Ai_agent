package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai_agent/internal/agent"
	"ai_agent/internal/analyzer"
	"ai_agent/internal/fixer"
	"ai_agent/internal/generator"
	"ai_agent/internal/llm"
	"ai_agent/internal/normalizer"
	"ai_agent/internal/report"
	"ai_agent/internal/scanner"
	"ai_agent/internal/spec"
	"ai_agent/internal/types"
	"ai_agent/internal/validator"
)

type App struct {
	ProjectPath  string
	SpecPath     string
	ReportPath   string
	ReportFormat string
	ReportMode   string
}

func (a *App) Analyze() (types.CommandResult, error) {
	sp, err := a.loadSpecOptional()
	if err != nil {
		return types.CommandResult{}, err
	}

	scan, baselineFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	baseline := validator.Run(a.ProjectPath, coverageGoal(sp))
	decisions := agent.NewPlanner().BuildPlan(sp, baselineFindings, scan.Functions)

	rep := types.Report{
		Project:            projectName(sp, a.ProjectPath),
		Spec:               sp,
		Functions:          scan.Functions,
		Findings:           baselineFindings,
		RemainingFindings:  baselineFindings,
		Decisions:          decisions,
		BaselineValidation: baseline,
		Validation:         baseline,
		Summary: buildSummary(
			baselineFindings,
			baselineFindings,
			nil,
			nil,
			baseline,
			baseline,
		),
	}

	rep.Summary.ManualReviewCount = countManualReviewDecisions(decisions)

	if err := a.writeReport(rep); err != nil {
		return types.CommandResult{}, err
	}

	return types.CommandResult{
		Command: "analyze",
		Report:  rep,
	}, nil
}

func (a *App) DryRun() (types.CommandResult, error) {
	sp, err := a.loadSpecRequired()
	if err != nil {
		return types.CommandResult{}, err
	}

	scan, baselineFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	baseline := validator.Run(a.ProjectPath, sp.TargetCoverage)
	decisions := agent.NewPlanner().BuildPlan(sp, baselineFindings, scan.Functions)

	plannedFixes := a.planFixes(decisions, baselineFindings)

	plannedSpec := *sp
	plannedSpec.Targets = agent.ExecutableTestTargets(decisions)
	plannedTests := generator.PlanTests(a.ProjectPath, plannedSpec, scan.Functions)

	rep := types.Report{
		Project:            sp.Name,
		Spec:               sp,
		Functions:          scan.Functions,
		Findings:           baselineFindings,
		RemainingFindings:  baselineFindings,
		Decisions:          decisions,
		Fixes:              plannedFixes,
		GeneratedTests:     plannedTests,
		BaselineValidation: baseline,
		Validation:         baseline,
		Summary: buildSummary(
			baselineFindings,
			baselineFindings,
			plannedFixes,
			plannedTests,
			baseline,
			baseline,
		),
	}

	rep.Summary.ManualReviewCount = countManualReviewDecisions(decisions)

	if err := a.writeReport(rep); err != nil {
		return types.CommandResult{}, err
	}

	return types.CommandResult{
		Command: "dry-run",
		Report:  rep,
	}, nil
}

func (a *App) Fix() (types.CommandResult, error) {
	sp, err := a.loadSpecOptional()
	if err != nil {
		return types.CommandResult{}, err
	}

	scan, baselineFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	baseline := validator.Run(a.ProjectPath, coverageGoal(sp))
	decisions := agent.NewPlanner().BuildPlan(sp, baselineFindings, scan.Functions)

	llmClient, llmErr := a.resolveLLMClient(sp)
	if llmPlanRequired(decisions) && llmErr != nil {
		return types.CommandResult{}, fmt.Errorf("llm is required by current plan: %w", llmErr)
	}
	fixes := a.applyFixes(decisions, baselineFindings, scan.Functions, coverageGoal(sp), llmClient, llmErr, sp)

	finalScan, remainingFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}
	finalValidation := validator.Run(a.ProjectPath, coverageGoal(sp))

	rep := types.Report{
		Project:            projectName(sp, a.ProjectPath),
		Spec:               sp,
		Functions:          finalScan.Functions,
		Findings:           baselineFindings,
		RemainingFindings:  remainingFindings,
		Decisions:          decisions,
		Fixes:              fixes,
		BaselineValidation: baseline,
		Validation:         finalValidation,
		Summary: buildSummary(
			baselineFindings,
			remainingFindings,
			fixes,
			nil,
			baseline,
			finalValidation,
		),
	}

	rep.Summary.ManualReviewCount = countManualReviewDecisions(decisions)

	if err := a.writeReport(rep); err != nil {
		return types.CommandResult{}, err
	}

	return types.CommandResult{
		Command: "fix",
		Report:  rep,
	}, nil
}

func (a *App) TestGen() (types.CommandResult, error) {
	sp, err := a.loadSpecRequired()
	if err != nil {
		return types.CommandResult{}, err
	}

	scan, baselineFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	baseline := validator.Run(a.ProjectPath, sp.TargetCoverage)
	decisions := agent.NewPlanner().BuildPlan(sp, baselineFindings, scan.Functions)

	filteredSpec := *sp
	filteredSpec.Targets = agent.ExecutableTestTargets(decisions)
	generatedTests := generator.GenerateTests(a.ProjectPath, filteredSpec, scan.Functions)

	finalValidation := validator.Run(a.ProjectPath, sp.TargetCoverage)
	if !finalValidation.BuildOK || !finalValidation.TestOK {
		generatedTests = rollbackGeneratedTests(generatedTests)
		finalValidation = validator.Run(a.ProjectPath, sp.TargetCoverage)
	}

	finalScan, remainingFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	rep := types.Report{
		Project:            sp.Name,
		Spec:               sp,
		Functions:          finalScan.Functions,
		Findings:           baselineFindings,
		RemainingFindings:  remainingFindings,
		Decisions:          decisions,
		GeneratedTests:     generatedTests,
		BaselineValidation: baseline,
		Validation:         finalValidation,
		Summary: buildSummary(
			baselineFindings,
			remainingFindings,
			nil,
			generatedTests,
			baseline,
			finalValidation,
		),
	}

	rep.Summary.ManualReviewCount = countManualReviewDecisions(decisions)

	if err := a.writeReport(rep); err != nil {
		return types.CommandResult{}, err
	}

	return types.CommandResult{
		Command: "testgen",
		Report:  rep,
	}, nil
}

func (a *App) Run() (types.CommandResult, error) {
	sp, err := a.loadSpecRequired()
	if err != nil {
		return types.CommandResult{}, err
	}

	scan, baselineFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	baseline := validator.Run(a.ProjectPath, sp.TargetCoverage)
	decisions := agent.NewPlanner().BuildPlan(sp, baselineFindings, scan.Functions)

	llmClient, llmErr := a.resolveLLMClient(sp)
	if llmPlanRequired(decisions) && llmErr != nil {
		return types.CommandResult{}, fmt.Errorf("llm is required by current plan: %w", llmErr)
	}

	fixes := a.applyFixes(decisions, baselineFindings, scan.Functions, sp.TargetCoverage, llmClient, llmErr, sp)

	executableTargets := agent.ExecutableTestTargets(decisions)
	eligibleTargets, skippedTests := a.partitionTestTargetsForRun(executableTargets, decisions, fixes, baselineFindings, scan.Functions)

	filteredSpec := *sp
	filteredSpec.Targets = eligibleTargets

	generatedTests := generator.GenerateTests(a.ProjectPath, filteredSpec, scan.Functions)
	generatedTests = append(generatedTests, skippedTests...)

	finalValidation := validator.Run(a.ProjectPath, sp.TargetCoverage)
	if !finalValidation.BuildOK || !finalValidation.TestOK {
		generatedTests = rollbackGeneratedTests(generatedTests)
		finalValidation = validator.Run(a.ProjectPath, sp.TargetCoverage)
	}

	finalScan, remainingFindings, err := a.analyzeCurrent()
	if err != nil {
		return types.CommandResult{}, err
	}

	rep := types.Report{
		Project:            sp.Name,
		Spec:               sp,
		Functions:          finalScan.Functions,
		Findings:           baselineFindings,
		RemainingFindings:  remainingFindings,
		Decisions:          decisions,
		Fixes:              fixes,
		GeneratedTests:     generatedTests,
		BaselineValidation: baseline,
		Validation:         finalValidation,
		Summary: buildSummary(
			baselineFindings,
			remainingFindings,
			fixes,
			generatedTests,
			baseline,
			finalValidation,
		),
	}

	rep.Summary.ManualReviewCount = countManualReviewDecisions(decisions)

	rep.Summary.ManualReviewCount = countManualReviewDecisions(decisions)

	if err := a.writeReport(rep); err != nil {
		return types.CommandResult{}, err
	}

	return types.CommandResult{
		Command: "run",
		Report:  rep,
	}, nil
}

func (a *App) ResetDemo() error {
	demoRoot := a.ProjectPath
	if demoRoot == "" {
		demoRoot = filepath.Join(".", "examples", "demo_project")
	}

	sourcePath := filepath.Join(demoRoot, "internal", "service", "user.go")
	source := `package service

import (
	"fmt"
	"os"
	"strings"
)

type User struct {
	Name string
}

func ValidateEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("empty email")
	}
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email")
	}
	return nil
}

func ReadFirstByte(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
	}

	buf := make([]byte, 1)
	_, err = f.Read(buf)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}

func UserLabel(u *User) (string, error) {
	return "User: " + u.Name, nil
}

func RemoveIfExists(path string) error {
	_ = os.Remove(path)
	return nil
}
`

	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		return fmt.Errorf("write demo source: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(demoRoot, "internal", "service", "*_ai_agent_test.go"))
	if err == nil {
		for _, p := range matches {
			_ = os.Remove(p)
		}
	}

	toRemove := []string{
		"coverage.out",
		"coverage.html",
		"build.log",
		"test.log",
		"race.log",
		"coverage.log",
		"fixes_report.json",
		"fixes_report.md",
		"analysis_report.json",
		"analysis_report.md",
		"testgen_report.json",
		"testgen_report.md",
		"dry_run_report.json",
		"dry_run_report.md",
		"dry_run_strict_report.json",
		"dry_run_strict_report.md",
		"dry_run_manual_review_report.json",
		"dry_run_manual_review_report.md",
		"eval_report.json",
		"eval_report.csv",
	}
	for _, name := range toRemove {
		_ = os.Remove(filepath.Join(demoRoot, name))
	}

	patterns := []string{
		filepath.Join(demoRoot, "eval_report_run_*.json"),
		filepath.Join(demoRoot, "eval_report_run_*.md"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err == nil {
			for _, p := range files {
				_ = os.Remove(p)
			}
		}
	}

	return nil
}

func (a *App) planFixes(decisions []types.AgentDecision, findings []types.Finding) []types.FixResult {
	findMap := map[string]types.Finding{}
	for _, f := range findings {
		findMap[f.ID] = f
	}

	var planned []types.FixResult
	for _, d := range decisions {
		if d.Kind != "finding" || !d.Executable {
			continue
		}

		fd, ok := findMap[d.TargetID]
		if !ok {
			continue
		}

		before, after := previewForFinding(fd, d.Action)
		strategy := strategyNameFromAction(d.Action)

		planned = append(planned, types.FixResult{
			FindingID:     fd.ID,
			File:          filepath.Join(a.ProjectPath, fd.File),
			Line:          fd.Line,
			Strategy:      strategy,
			Description:   d.Description,
			BeforeSnippet: before,
			AfterSnippet:  after,
			Planned:       true,
		})
	}

	return planned
}

func (a *App) applyFixes(
	decisions []types.AgentDecision,
	findings []types.Finding,
	functions []types.FunctionInfo,
	coverageGoal float64,
	llmClient llm.Client,
	llmErr error,
	sp *types.Spec,
) []types.FixResult {
	templateFixers := []fixer.Fixer{
		fixer.NewIgnoredErrorTemplateFixer(),
		fixer.NewErrReturnTemplateFixer(),
		fixer.NewDeferCloseTemplateFixer(),
		fixer.NewNilGuardTemplateFixer(),
	}
	llmFixers := []fixer.Fixer{
		fixer.NewLLMPatchFixer(llmClient, llmErr),
	}

	decisionMap := agent.DecisionIndex(decisions)
	allowLLMFallback := sp != nil && sp.LLM.Enabled && sp.AgentPolicy.LLMFallbackOnTemplateFailure

	var fixes []types.FixResult

	for _, fd := range findings {
		d, ok := decisionMap[fd.ID]
		if !ok || !d.Executable {
			continue
		}

		switch d.Action {
		case "apply_template_fix":
			templateRes, handled := a.tryFixers(templateFixers, fd, functions, coverageGoal)
			if handled {
				fixes = append(fixes, templateRes)
			}

			if allowLLMFallback && (!handled || !templateRes.Applied || !templateRes.ValidationOK) {
				llmRes, llmHandled := a.tryFixers(llmFixers, fd, functions, coverageGoal)
				if llmHandled {
					fixes = append(fixes, llmRes)
				}
			}

		case "apply_llm_fix":
			llmRes, llmHandled := a.tryFixers(llmFixers, fd, functions, coverageGoal)
			if llmHandled {
				fixes = append(fixes, llmRes)
			}
		}
	}

	return fixes
}

func (a *App) tryFixers(
	fixers []fixer.Fixer,
	fd types.Finding,
	functions []types.FunctionInfo,
	coverageGoal float64,
) (types.FixResult, bool) {
	for _, fx := range fixers {
		if !fx.CanApply(fd) {
			continue
		}

		fixResult, err := fx.Apply(a.ProjectPath, fd, functions)
		if err != nil {
			fixResult.Error = err.Error()
			if fixResult.Strategy == "" {
				fixResult.Strategy = fx.Name()
			}
			return fixResult, true
		}

		if !fixResult.Applied {
			if fixResult.Strategy == "" {
				fixResult.Strategy = fx.Name()
			}
			return fixResult, true
		}

		validation := validator.Run(a.ProjectPath, coverageGoal)
		if validation.BuildOK && validation.TestOK {
			fixResult.ValidationOK = true
			if err := validator.CleanupBackup(fixResult.File); err != nil && fixResult.Error == "" {
				fixResult.Error = "applied, but backup cleanup failed: " + err.Error()
			}
			return fixResult, true
		}

		if err := validator.RollbackFromBackup(fixResult.File); err != nil {
			fixResult.Error = "validation failed and rollback failed: " + err.Error()
		} else {
			fixResult.RolledBack = true
			fixResult.Error = "rolled back after failed validation"
		}

		if err := validator.CleanupBackup(fixResult.File); err != nil && fixResult.Error == "" {
			fixResult.Error = "rollback completed, but backup cleanup failed: " + err.Error()
		}

		return fixResult, true
	}

	return types.FixResult{}, false
}

func (a *App) analyzeCurrent() (*scanner.ScanResult, []types.Finding, error) {
	scan, err := scanner.ScanProject(a.ProjectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("scan project: %w", err)
	}

	rawFindings, err := analyzer.RunAll(
		a.ProjectPath,
		analyzer.NewGoVetAnalyzer(),
		analyzer.NewStaticcheckAnalyzer(),
		analyzer.NewASTIgnoredErrorAnalyzer(),
		analyzer.NewASTMissingCloseAnalyzer(),
		analyzer.NewASTWeakErrHandlerAnalyzer(),
		analyzer.NewASTNilGuardAnalyzer(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("run analyzers: %w", err)
	}

	findings := normalizer.Normalize(rawFindings)
	return scan, findings, nil
}

func (a *App) loadSpecOptional() (*types.Spec, error) {
	if a.SpecPath == "" {
		return nil, nil
	}

	sp, err := spec.Load(a.SpecPath)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	return sp, nil
}

func (a *App) loadSpecRequired() (*types.Spec, error) {
	if a.SpecPath == "" {
		return nil, fmt.Errorf("spec is required")
	}

	sp, err := spec.Load(a.SpecPath)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	return sp, nil
}

func (a *App) resolveLLMClient(sp *types.Spec) (llm.Client, error) {
	return llm.NewFromSpec(sp, a.SpecPath)
}

func (a *App) writeReport(rep types.Report) error {
	if a.ReportPath == "" {
		return nil
	}
	return report.Write(a.ReportPath, rep, a.ReportFormat, a.ReportMode)
}

func rollbackGeneratedTests(tests []types.TestGenerationResult) []types.TestGenerationResult {
	fileToIndexes := map[string][]int{}

	for i := range tests {
		if !tests[i].Generated || tests[i].File == "" {
			continue
		}
		fileToIndexes[tests[i].File] = append(fileToIndexes[tests[i].File], i)
	}

	for file, indexes := range fileToIndexes {
		err := os.Remove(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			for _, idx := range indexes {
				tests[idx].Generated = false
				if tests[idx].Error == "" {
					tests[idx].Error = "generated test failed validation and could not be removed: " + err.Error()
				} else {
					tests[idx].Error += "; cleanup failed: " + err.Error()
				}
			}
			continue
		}

		for _, idx := range indexes {
			tests[idx].Generated = false
			if tests[idx].Error == "" {
				tests[idx].Error = "rolled back after failed validation"
			} else {
				tests[idx].Error += "; rolled back after failed validation"
			}
		}
	}

	return tests
}

func projectName(sp *types.Spec, projectPath string) string {
	if sp != nil && sp.Name != "" {
		return sp.Name
	}
	return projectPath
}

func coverageGoal(sp *types.Spec) float64 {
	if sp == nil {
		return 0
	}
	return sp.TargetCoverage
}

func strategyNameFromAction(action string) string {
	switch action {
	case "apply_template_fix":
		return "template"
	case "apply_llm_fix":
		return "llm"
	default:
		return action
	}
}

func previewForFinding(fd types.Finding, action string) (string, string) {
	if action == "apply_llm_fix" {
		return "// current function body", "// llm-generated patched function body"
	}

	switch {
	case strings.HasPrefix(fd.ID, "ast_ignored_error:"):
		return "_ = someCall()", "if err := someCall(); err != nil {\n\treturn err\n}"
	case fd.Category == "resource_management" && fd.FixCategory == "A":
		return `f, err := os.Open(path)`, "f, err := os.Open(path)\nif err != nil {\n\treturn \"\", err\n}\ndefer f.Close()"
	case fd.Category == "error_handling" && fd.FixCategory == "A":
		return "if err != nil {\n\tfmt.Println(err)\n}", "if err != nil {\n\treturn \"\", err\n}"
	case fd.Category == "nil_safety" && fd.FixCategory == "B":
		param := fd.Symbol
		if param == "" {
			param = "arg"
		}
		return "// pointer parameter used without nil guard", "if " + param + " == nil {\n\treturn \"\", fmt.Errorf(\"" + param + " is nil\")\n}"
	default:
		return "", ""
	}
}

func buildSummary(
	baselineFindings []types.Finding,
	remainingFindings []types.Finding,
	fixes []types.FixResult,
	tests []types.TestGenerationResult,
	baseline types.ValidationResult,
	final types.ValidationResult,
) types.SummaryMetrics {
	var s types.SummaryMetrics

	s.FindingsTotal = len(baselineFindings)
	s.RemainingFindings = len(remainingFindings)

	resolved := len(baselineFindings) - len(remainingFindings)
	if resolved < 0 {
		resolved = 0
	}
	s.FindingsResolved = resolved

	s.FixesAttempted = len(fixes)

	for _, f := range fixes {
		finalApplied := f.Applied && f.ValidationOK && !f.RolledBack

		if finalApplied {
			s.FixesApplied++
		}
		if f.RolledBack {
			s.FixesRolledBack++
		}
		if f.Planned {
			s.PlannedFixes++
		}

		switch f.Strategy {
		case "template":
			if finalApplied {
				s.TemplateApplied++
			}
		case "llm":
			s.LLMAttempted++
			if finalApplied {
				s.LLMApplied++
			} else {
				s.LLMFailed++
			}
		}
	}

	for _, t := range tests {
		if t.Generated {
			s.TestsGenerated++
		}
		if t.Planned {
			s.PlannedTests++
		}
		if !t.Generated && t.Error != "" {
			s.SkippedTests++
		}
	}

	s.CoverageBefore = baseline.Coverage
	s.CoverageAfter = final.Coverage
	s.CoverageDelta = final.Coverage - baseline.Coverage

	return s
}

func llmPlanRequired(decisions []types.AgentDecision) bool {
	for _, d := range decisions {
		if d.Kind == "finding" && d.Action == "apply_llm_fix" && d.Executable {
			return true
		}
	}
	return false
}

func (a *App) partitionTestTargetsForRun(
	targets []string,
	decisions []types.AgentDecision,
	fixes []types.FixResult,
	findings []types.Finding,
	functions []types.FunctionInfo,
) ([]string, []types.TestGenerationResult) {
	blockedTargets := blockedTargetsForRun(decisions, fixes, findings, functions)

	var eligible []string
	var skipped []types.TestGenerationResult

	for _, target := range targets {
		normalized := normalizeTargetForCompare(target)
		if reason, blocked := blockedTargets[normalized]; blocked {
			skipped = append(skipped, types.TestGenerationResult{
				Target:      normalized,
				File:        buildTestFilePathForTarget(a.ProjectPath, normalized),
				Generated:   false,
				Description: reason,
				Error:       reason,
			})
			continue
		}

		eligible = append(eligible, target)
	}

	return eligible, skipped
}

func failedFixTargets(
	fixes []types.FixResult,
	findings []types.Finding,
	functions []types.FunctionInfo,
) map[string]bool {
	findingToTarget := map[string]string{}

	for _, fd := range findings {
		if target, ok := functionTargetForFinding(fd, functions); ok {
			findingToTarget[fd.ID] = target
		}
	}

	blocked := map[string]bool{}
	for _, fx := range fixes {
		if fx.Applied && fx.ValidationOK && !fx.RolledBack {
			continue
		}
		if target, ok := findingToTarget[fx.FindingID]; ok {
			blocked[normalizeTargetForCompare(target)] = true
		}
	}

	return blocked
}

func functionTargetForFinding(fd types.Finding, functions []types.FunctionInfo) (string, bool) {
	targetFile := normalizePathForCompare(fd.File)

	bestLine := -1
	var best types.FunctionInfo

	for _, fn := range functions {
		if normalizePathForCompare(fn.File) != targetFile {
			continue
		}
		if fn.Line <= fd.Line && fn.Line > bestLine {
			best = fn
			bestLine = fn.Line
		}
	}

	if bestLine == -1 {
		return "", false
	}

	return normalizePathForCompare(best.File) + ":" + best.Name, true
}

func normalizeTargetForCompare(target string) string {
	parts := strings.Split(target, ":")
	if len(parts) != 2 {
		return normalizePathForCompare(target)
	}
	return normalizePathForCompare(parts[0]) + ":" + parts[1]
}

func normalizePathForCompare(p string) string {
	p = filepath.Clean(p)
	return strings.ReplaceAll(p, "\\", "/")
}

func buildTestFilePathForTarget(projectRoot, target string) string {
	parts := strings.Split(target, ":")
	if len(parts) != 2 {
		return ""
	}

	sourceFile := filepath.FromSlash(parts[0])
	if !filepath.IsAbs(sourceFile) {
		sourceFile = filepath.Join(projectRoot, sourceFile)
	}

	abs, err := filepath.Abs(sourceFile)
	if err == nil {
		sourceFile = abs
	}

	base := filepath.Base(sourceFile)
	name := strings.TrimSuffix(base, ".go") + "_ai_agent_test.go"
	return filepath.Join(filepath.Dir(sourceFile), name)
}

func countManualReviewDecisions(decisions []types.AgentDecision) int {
	n := 0
	for _, d := range decisions {
		if d.Kind == "finding" && d.Action == "manual_review" {
			n++
		}
	}
	return n
}

func blockedTargetsForRun(
	decisions []types.AgentDecision,
	fixes []types.FixResult,
	findings []types.Finding,
	functions []types.FunctionInfo,
) map[string]string {
	blocked := map[string]string{}

	findingByID := map[string]types.Finding{}
	for _, fd := range findings {
		findingByID[fd.ID] = fd
	}

	findingToTarget := map[string]string{}
	for _, fd := range findings {
		if target, ok := functionTargetForFinding(fd, functions); ok {
			findingToTarget[fd.ID] = normalizeTargetForCompare(target)
		}
	}

	// 1. Блокируем targets, где fix не был финально применён.
	for _, fx := range fixes {
		finalApplied := fx.Applied && fx.ValidationOK && !fx.RolledBack
		if finalApplied {
			continue
		}
		if target, ok := findingToTarget[fx.FindingID]; ok {
			blocked[target] = "skipped test generation because related fix failed or was not validated"
		}
	}

	// 2. Блокируем targets, где finding ушёл в manual_review или skip.
	for _, d := range decisions {
		if d.Kind != "finding" {
			continue
		}

		fd, ok := findingByID[d.TargetID]
		if !ok {
			continue
		}

		target, ok := functionTargetForFinding(fd, functions)
		if !ok {
			continue
		}
		target = normalizeTargetForCompare(target)

		switch d.Action {
		case "manual_review":
			if _, exists := blocked[target]; !exists {
				blocked[target] = "skipped test generation because related finding requires manual review"
			}
		case "skip":
			if _, exists := blocked[target]; !exists {
				blocked[target] = "skipped test generation because related finding is not executable under current policy"
			}
		}
	}

	return blocked
}
