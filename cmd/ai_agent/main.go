package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"ai_agent/internal/app"
	evaluator "ai_agent/internal/eval"
	explainpkg "ai_agent/internal/explain"
	"ai_agent/internal/types"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "analyze":
		runAnalyze(os.Args[2:])
	case "dry-run":
		runDryRun(os.Args[2:])
	case "fix":
		runFix(os.Args[2:])
	case "testgen":
		runTestGen(os.Args[2:])
	case "run":
		runRun(os.Args[2:])
	case "eval":
		runEval(os.Args[2:])
	case "explain":
		runExplain(os.Args[2:])
	case "reset-demo":
		runResetDemo(os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}

func runAnalyze(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)

	var projectPath, specPath, reportPath, reportFormat, reportMode string

	fs.StringVar(&projectPath, "project", ".", "path to Go project")
	fs.StringVar(&specPath, "spec", "", "optional path to spec.yaml")
	fs.StringVar(&reportPath, "report", "./analysis_report.json", "path to output report")
	fs.StringVar(&reportFormat, "report-format", "json", "report format: json | md | both")
	fs.StringVar(&reportMode, "report-mode", "compact", "report mode: compact | full")

	_ = fs.Parse(args)

	a := app.App{
		ProjectPath:  projectPath,
		SpecPath:     specPath,
		ReportPath:   reportPath,
		ReportFormat: reportFormat,
		ReportMode:   reportMode,
	}

	res, err := a.Analyze()
	if err != nil {
		log.Fatalf("analyze failed: %v", err)
	}

	printSummary(res)
}

func runDryRun(args []string) {
	fs := flag.NewFlagSet("dry-run", flag.ExitOnError)

	var projectPath, specPath, reportPath, reportFormat, reportMode string

	fs.StringVar(&projectPath, "project", ".", "path to Go project")
	fs.StringVar(&specPath, "spec", "./examples/spec.yaml", "path to spec.yaml")
	fs.StringVar(&reportPath, "report", "./dry_run_report.json", "path to output report")
	fs.StringVar(&reportFormat, "report-format", "json", "report format: json | md | both")
	fs.StringVar(&reportMode, "report-mode", "compact", "report mode: compact | full")

	_ = fs.Parse(args)

	a := app.App{
		ProjectPath:  projectPath,
		SpecPath:     specPath,
		ReportPath:   reportPath,
		ReportFormat: reportFormat,
		ReportMode:   reportMode,
	}

	res, err := a.DryRun()
	if err != nil {
		log.Fatalf("dry-run failed: %v", err)
	}

	printSummary(res)
}

func runFix(args []string) {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)

	var projectPath, specPath, reportPath, reportFormat, reportMode string

	fs.StringVar(&projectPath, "project", ".", "path to Go project")
	fs.StringVar(&specPath, "spec", "./examples/spec.yaml", "path to spec.yaml")
	fs.StringVar(&reportPath, "report", "./fixes_report.json", "path to output report")
	fs.StringVar(&reportFormat, "report-format", "json", "report format: json | md | both")
	fs.StringVar(&reportMode, "report-mode", "compact", "report mode: compact | full")

	_ = fs.Parse(args)

	a := app.App{
		ProjectPath:  projectPath,
		SpecPath:     specPath,
		ReportPath:   reportPath,
		ReportFormat: reportFormat,
		ReportMode:   reportMode,
	}

	res, err := a.Fix()
	if err != nil {
		log.Fatalf("fix failed: %v", err)
	}

	printSummary(res)
}

func runTestGen(args []string) {
	fs := flag.NewFlagSet("testgen", flag.ExitOnError)

	var projectPath, specPath, reportPath, reportFormat, reportMode string

	fs.StringVar(&projectPath, "project", ".", "path to Go project")
	fs.StringVar(&specPath, "spec", "./examples/spec.yaml", "path to spec.yaml")
	fs.StringVar(&reportPath, "report", "./testgen_report.json", "path to output report")
	fs.StringVar(&reportFormat, "report-format", "json", "report format: json | md | both")
	fs.StringVar(&reportMode, "report-mode", "compact", "report mode: compact | full")

	_ = fs.Parse(args)

	a := app.App{
		ProjectPath:  projectPath,
		SpecPath:     specPath,
		ReportPath:   reportPath,
		ReportFormat: reportFormat,
		ReportMode:   reportMode,
	}

	res, err := a.TestGen()
	if err != nil {
		log.Fatalf("testgen failed: %v", err)
	}

	printSummary(res)
}

func runRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)

	var projectPath, specPath, reportPath, reportFormat, reportMode string

	fs.StringVar(&projectPath, "project", ".", "path to Go project")
	fs.StringVar(&specPath, "spec", "./examples/spec.yaml", "path to spec.yaml")
	fs.StringVar(&reportPath, "report", "./fixes_report.json", "path to output report")
	fs.StringVar(&reportFormat, "report-format", "json", "report format: json | md | both")
	fs.StringVar(&reportMode, "report-mode", "compact", "report mode: compact | full")

	_ = fs.Parse(args)

	a := app.App{
		ProjectPath:  projectPath,
		SpecPath:     specPath,
		ReportPath:   reportPath,
		ReportFormat: reportFormat,
		ReportMode:   reportMode,
	}

	res, err := a.Run()
	if err != nil {
		log.Fatalf("run failed: %v", err)
	}

	printSummary(res)
}

func runEval(args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)

	var projectPath, specPath, command, outPath string
	var runs int
	var resetBefore bool

	fs.StringVar(&projectPath, "project", "./examples/demo_project", "path to Go project")
	fs.StringVar(&specPath, "spec", "./examples/spec.yaml", "path to spec.yaml")
	fs.StringVar(&command, "command", "run", "command to evaluate: analyze | dry-run | fix | testgen | run")
	fs.StringVar(&outPath, "report", "./eval_report.json", "path to aggregate evaluation report")
	fs.IntVar(&runs, "runs", 3, "number of evaluation runs")
	fs.BoolVar(&resetBefore, "reset-before", true, "reset demo project before each run")

	_ = fs.Parse(args)

	agg, err := evaluator.Execute(projectPath, specPath, command, runs, resetBefore, outPath)
	if err != nil {
		log.Fatalf("eval failed: %v", err)
	}

	fmt.Printf("Command: eval\n")
	fmt.Printf("Project: %s\n", agg.Project)
	fmt.Printf("Spec: %s\n", agg.Spec)
	fmt.Printf("Evaluated command: %s\n", agg.Command)
	fmt.Printf("Runs completed: %d/%d\n", agg.RunsCompleted, agg.RunsRequested)
	fmt.Printf("All runs OK: %v\n", agg.AllRunsOK)
	fmt.Printf("Average duration: %.2f ms\n", agg.AvgDurationMs)
	fmt.Printf("Average coverage: %.2f%%\n", agg.AvgCoverage)
	fmt.Printf("Min coverage: %.2f%% | Max coverage: %.2f%%\n", agg.MinCoverage, agg.MaxCoverage)
	fmt.Printf("Aggregate report: %s\n", outPath)
}

func runExplain(args []string) {
	fs := flag.NewFlagSet("explain", flag.ExitOnError)

	var reportPath string

	fs.StringVar(&reportPath, "report", "./fixes_report.json", "path to report json")
	_ = fs.Parse(args)

	rep, err := explainpkg.LoadReport(reportPath)
	if err != nil {
		log.Fatalf("explain failed: %v", err)
	}

	fmt.Print(explainpkg.Render(rep))
}

func runResetDemo(args []string) {
	fs := flag.NewFlagSet("reset-demo", flag.ExitOnError)

	var projectPath string

	fs.StringVar(&projectPath, "project", "./examples/demo_project", "path to demo project")
	_ = fs.Parse(args)

	a := app.App{
		ProjectPath: projectPath,
	}

	if err := a.ResetDemo(); err != nil {
		log.Fatalf("reset-demo failed: %v", err)
	}

	fmt.Printf("Demo project reset: %s\n", projectPath)
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  ai_agent analyze    --project ./demo [--spec ./spec.yaml] [--report ./analysis_report.json] [--report-format json|md|both] [--report-mode compact|full]")
	fmt.Println("  ai_agent dry-run    --project ./demo --spec ./spec.yaml [--report ./dry_run_report.json] [--report-format json|md|both] [--report-mode compact|full]")
	fmt.Println("  ai_agent fix        --project ./demo --spec ./spec.yaml [--report ./fixes_report.json] [--report-format json|md|both] [--report-mode compact|full]")
	fmt.Println("  ai_agent testgen    --project ./demo --spec ./spec.yaml [--report ./testgen_report.json] [--report-format json|md|both] [--report-mode compact|full]")
	fmt.Println("  ai_agent run        --project ./demo --spec ./spec.yaml [--report ./fixes_report.json] [--report-format json|md|both] [--report-mode compact|full]")
	fmt.Println("  ai_agent eval       --project ./demo --spec ./spec.yaml [--command run] [--runs 3] [--reset-before=true] [--report ./eval_report.json]")
	fmt.Println("  ai_agent explain    --report ./fixes_report.json")
	fmt.Println("  ai_agent reset-demo --project ./examples/demo_project")
}

func printSummary(res types.CommandResult) {
	fixesAttempted := len(res.Report.Fixes)
	fixesApplied := countAppliedFixes(res.Report.Fixes)
	testsGenerated := countGeneratedTests(res.Report.GeneratedTests)
	testsPlanned := countPlannedTests(res.Report.GeneratedTests)

	templateApplied := res.Report.Summary.TemplateApplied
	llmAttempted := res.Report.Summary.LLMAttempted
	llmApplied := res.Report.Summary.LLMApplied
	llmFailed := res.Report.Summary.LLMFailed
	manualReview := res.Report.Summary.ManualReviewCount
	skippedTests := res.Report.Summary.SkippedTests

	fmt.Printf("Command: %s\n", res.Command)
	fmt.Printf("Project: %s\n", res.Report.Project)
	fmt.Printf("Functions found: %d\n", len(res.Report.Functions))
	fmt.Printf("Findings found: %d | Resolved: %d | Remaining: %d\n",
		len(res.Report.Findings),
		res.Report.Summary.FindingsResolved,
		res.Report.Summary.RemainingFindings,
	)
	fmt.Printf("Fix attempts: %d | Fixes applied: %d | Rolled back: %d\n",
		fixesAttempted,
		fixesApplied,
		res.Report.Summary.FixesRolledBack,
	)
	fmt.Printf("Template applied: %d | LLM attempted: %d | LLM applied: %d | LLM failed: %d | Manual review: %d\n",
		templateApplied,
		llmAttempted,
		llmApplied,
		llmFailed,
		manualReview,
	)
	fmt.Printf("Generated tests: %d | Planned tests: %d | Skipped tests: %d\n",
		testsGenerated,
		testsPlanned,
		skippedTests,
	)
	fmt.Printf(
		"Build OK: %v | Test OK: %v | Race: %s | Coverage: %.2f%%\n",
		res.Report.Validation.BuildOK,
		res.Report.Validation.TestOK,
		res.Report.Validation.RaceStatus,
		res.Report.Validation.Coverage,
	)
}

func countAppliedFixes(fixes []types.FixResult) int {
	n := 0
	for _, f := range fixes {
		if f.Applied && f.ValidationOK && !f.RolledBack {
			n++
		}
	}
	return n
}

func countGeneratedTests(tests []types.TestGenerationResult) int {
	n := 0
	for _, t := range tests {
		if t.Generated {
			n++
		}
	}
	return n
}

func countPlannedTests(tests []types.TestGenerationResult) int {
	n := 0
	for _, t := range tests {
		if t.Planned {
			n++
		}
	}
	return n
}
