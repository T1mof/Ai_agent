package eval

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai_agent/internal/app"
	"ai_agent/internal/types"
)

type RunMetrics struct {
	RunIndex          int     `json:"run_index"`
	Command           string  `json:"command"`
	DurationMs        int64   `json:"duration_ms"`
	FindingsTotal     int     `json:"findings_total"`
	RemainingFindings int     `json:"remaining_findings"`
	FindingsResolved  int     `json:"findings_resolved"`
	FixesApplied      int     `json:"fixes_applied"`
	TestsGenerated    int     `json:"tests_generated"`
	BuildOK           bool    `json:"build_ok"`
	TestOK            bool    `json:"test_ok"`
	RaceOK            bool    `json:"race_ok"`
	RaceSupported     bool    `json:"race_supported"`
	RaceStatus        string  `json:"race_status"`
	CoverageBefore    float64 `json:"coverage_before"`
	CoverageAfter     float64 `json:"coverage_after"`
	CoverageGoal      float64 `json:"coverage_goal"`
	CoverageMet       bool    `json:"coverage_met"`
	ReportPath        string  `json:"report_path"`
	Error             string  `json:"error,omitempty"`
}

type AggregateReport struct {
	Project        string       `json:"project"`
	Spec           string       `json:"spec"`
	Command        string       `json:"command"`
	RunsRequested  int          `json:"runs_requested"`
	RunsCompleted  int          `json:"runs_completed"`
	ResetBeforeRun bool         `json:"reset_before_run"`
	AvgDurationMs  float64      `json:"avg_duration_ms"`
	AvgCoverage    float64      `json:"avg_coverage"`
	MinCoverage    float64      `json:"min_coverage"`
	MaxCoverage    float64      `json:"max_coverage"`
	AllRunsOK      bool         `json:"all_runs_ok"`
	Results        []RunMetrics `json:"results"`
}

func Execute(projectPath, specPath, command string, runs int, resetBefore bool, outPath string) (*AggregateReport, error) {
	if runs <= 0 {
		return nil, fmt.Errorf("runs must be > 0")
	}

	agg := &AggregateReport{
		Project:        projectPath,
		Spec:           specPath,
		Command:        command,
		RunsRequested:  runs,
		ResetBeforeRun: resetBefore,
		AllRunsOK:      true,
	}

	var totalDuration int64
	var totalCoverage float64
	var minCoverage float64
	var maxCoverage float64

	for i := 0; i < runs; i++ {
		if resetBefore {
			resetApp := app.App{ProjectPath: projectPath}
			if err := resetApp.ResetDemo(); err != nil {
				metric := RunMetrics{
					RunIndex: i + 1,
					Command:  command,
					Error:    "reset-demo failed: " + err.Error(),
				}
				agg.Results = append(agg.Results, metric)
				agg.AllRunsOK = false
				continue
			}
		}

		runReportPath := buildRunReportPath(outPath, i+1)
		runApp := app.App{
			ProjectPath:  projectPath,
			SpecPath:     specPath,
			ReportPath:   runReportPath,
			ReportFormat: "json",
			ReportMode:   "compact",
		}

		start := time.Now()
		res, err := executeCommand(runApp, command)
		duration := time.Since(start).Milliseconds()

		metric := RunMetrics{
			RunIndex:   i + 1,
			Command:    command,
			DurationMs: duration,
			ReportPath: runReportPath,
		}

		if err != nil {
			metric.Error = err.Error()
			agg.Results = append(agg.Results, metric)
			agg.AllRunsOK = false
			continue
		}

		metric.FindingsTotal = res.Report.Summary.FindingsTotal
		metric.RemainingFindings = res.Report.Summary.RemainingFindings
		metric.FindingsResolved = res.Report.Summary.FindingsResolved
		metric.FixesApplied = res.Report.Summary.FixesApplied
		metric.TestsGenerated = res.Report.Summary.TestsGenerated
		metric.BuildOK = res.Report.Validation.BuildOK
		metric.TestOK = res.Report.Validation.TestOK
		metric.RaceOK = res.Report.Validation.RaceOK
		metric.RaceSupported = res.Report.Validation.RaceSupported
		metric.RaceStatus = res.Report.Validation.RaceStatus
		metric.CoverageBefore = res.Report.Summary.CoverageBefore
		metric.CoverageAfter = res.Report.Summary.CoverageAfter
		metric.CoverageGoal = res.Report.Validation.CoverageGoal
		metric.CoverageMet = res.Report.Validation.CoverageMet

		if !metric.BuildOK || !metric.TestOK {
			agg.AllRunsOK = false
		}
		if metric.RaceSupported && !metric.RaceOK {
			agg.AllRunsOK = false
		}

		if agg.RunsCompleted == 0 {
			minCoverage = metric.CoverageAfter
			maxCoverage = metric.CoverageAfter
		} else {
			if metric.CoverageAfter < minCoverage {
				minCoverage = metric.CoverageAfter
			}
			if metric.CoverageAfter > maxCoverage {
				maxCoverage = metric.CoverageAfter
			}
		}

		totalDuration += duration
		totalCoverage += metric.CoverageAfter
		agg.RunsCompleted++
		agg.Results = append(agg.Results, metric)
	}

	if agg.RunsCompleted > 0 {
		agg.AvgDurationMs = float64(totalDuration) / float64(agg.RunsCompleted)
		agg.AvgCoverage = totalCoverage / float64(agg.RunsCompleted)
		agg.MinCoverage = minCoverage
		agg.MaxCoverage = maxCoverage
	}

	if err := writeAggregate(outPath, agg); err != nil {
		return nil, err
	}

	return agg, nil
}

func executeCommand(a app.App, command string) (types.CommandResult, error) {
	switch command {
	case "analyze":
		return a.Analyze()
	case "dry-run":
		return a.DryRun()
	case "fix":
		return a.Fix()
	case "testgen":
		return a.TestGen()
	case "run":
		return a.Run()
	default:
		return types.CommandResult{}, fmt.Errorf("unsupported eval command: %s", command)
	}
}

func buildRunReportPath(base string, runIndex int) string {
	ext := filepath.Ext(base)
	if ext == "" {
		ext = ".json"
	}
	prefix := strings.TrimSuffix(base, ext)
	return fmt.Sprintf("%s_run_%02d%s", prefix, runIndex, ext)
}

func writeAggregate(path string, agg *AggregateReport) error {
	data, err := json.MarshalIndent(agg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	csvPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".csv"
	if err := writeAggregateCSV(csvPath, agg); err != nil {
		return err
	}

	return nil
}

func writeAggregateCSV(path string, agg *AggregateReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)

	header := []string{
		"run_index",
		"command",
		"duration_ms",
		"findings_total",
		"remaining_findings",
		"findings_resolved",
		"fixes_applied",
		"tests_generated",
		"build_ok",
		"test_ok",
		"race_supported",
		"race_ok",
		"race_status",
		"coverage_before",
		"coverage_after",
		"coverage_goal",
		"coverage_met",
		"report_path",
		"error",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, r := range agg.Results {
		row := []string{
			strconv.Itoa(r.RunIndex),
			r.Command,
			strconv.FormatInt(r.DurationMs, 10),
			strconv.Itoa(r.FindingsTotal),
			strconv.Itoa(r.RemainingFindings),
			strconv.Itoa(r.FindingsResolved),
			strconv.Itoa(r.FixesApplied),
			strconv.Itoa(r.TestsGenerated),
			strconv.FormatBool(r.BuildOK),
			strconv.FormatBool(r.TestOK),
			strconv.FormatBool(r.RaceSupported),
			strconv.FormatBool(r.RaceOK),
			r.RaceStatus,
			fmt.Sprintf("%.2f", r.CoverageBefore),
			fmt.Sprintf("%.2f", r.CoverageAfter),
			fmt.Sprintf("%.2f", r.CoverageGoal),
			strconv.FormatBool(r.CoverageMet),
			r.ReportPath,
			r.Error,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}
