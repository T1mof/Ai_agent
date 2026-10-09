package explain

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"ai_agent/internal/types"
)

func LoadReport(path string) (types.Report, error) {
	var rep types.Report

	data, err := os.ReadFile(path)
	if err != nil {
		return rep, fmt.Errorf("read report: %w", err)
	}

	if err := json.Unmarshal(data, &rep); err != nil {
		return rep, fmt.Errorf("parse report json: %w", err)
	}

	return rep, nil
}

func Render(rep types.Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== AI Agent Explain ===\n")
	fmt.Fprintf(&b, "Project: %s\n", rep.Project)
	fmt.Fprintf(&b, "Coverage: %.2f%% (goal: %.2f%%, met: %v)\n",
		rep.Validation.Coverage,
		rep.Validation.CoverageGoal,
		rep.Validation.CoverageMet,
	)
	fmt.Fprintf(&b, "Build: %s | Tests: %s | Race: %s\n",
		boolLabel(rep.Validation.BuildOK),
		boolLabel(rep.Validation.TestOK),
		rep.Validation.RaceStatus,
	)

	status := "SUCCESS"
	if !rep.Validation.BuildOK || !rep.Validation.TestOK {
		status = "FAILED"
	} else if rep.Summary.RemainingFindings > 0 {
		status = "PARTIAL"
	}
	fmt.Fprintf(&b, "Overall status: %s\n", status)

	fmt.Fprintf(&b, "\n-- Summary --\n")
	fmt.Fprintf(&b, "Findings total: %d\n", rep.Summary.FindingsTotal)
	fmt.Fprintf(&b, "Resolved: %d\n", rep.Summary.FindingsResolved)
	fmt.Fprintf(&b, "Remaining: %d\n", rep.Summary.RemainingFindings)
	fmt.Fprintf(&b, "Fix attempts: %d\n", rep.Summary.FixesAttempted)
	fmt.Fprintf(&b, "Fixes applied: %d\n", rep.Summary.FixesApplied)
	fmt.Fprintf(&b, "Rolled back: %d\n", rep.Summary.FixesRolledBack)
	fmt.Fprintf(&b, "Template applied: %d\n", rep.Summary.TemplateApplied)
	fmt.Fprintf(&b, "LLM attempted: %d\n", rep.Summary.LLMAttempted)
	fmt.Fprintf(&b, "LLM applied: %d\n", rep.Summary.LLMApplied)
	fmt.Fprintf(&b, "LLM failed: %d\n", rep.Summary.LLMFailed)
	fmt.Fprintf(&b, "Manual review: %d\n", rep.Summary.ManualReviewCount)
	fmt.Fprintf(&b, "Generated tests: %d\n", rep.Summary.TestsGenerated)
	fmt.Fprintf(&b, "Skipped tests: %d\n", rep.Summary.SkippedTests)

	findingsByID := make(map[string]types.Finding, len(rep.Findings))
	for _, fd := range rep.Findings {
		findingsByID[fd.ID] = fd
	}

	if len(rep.Fixes) > 0 {
		fmt.Fprintf(&b, "\n-- Applied / attempted fixes --\n")

		fixes := append([]types.FixResult(nil), rep.Fixes...)
		sort.Slice(fixes, func(i, j int) bool {
			if fixes[i].Line == fixes[j].Line {
				return fixes[i].FindingID < fixes[j].FindingID
			}
			return fixes[i].Line < fixes[j].Line
		})

		for _, fx := range fixes {
			fd, ok := findingsByID[fx.FindingID]
			msg := fx.FindingID
			if ok && fd.Message != "" {
				msg = fd.Message
			}

			fmt.Fprintf(&b, "- [%s] %s\n", strings.ToUpper(fx.Strategy), fixStatus(fx))
			fmt.Fprintf(&b, "  Finding: %s\n", msg)
			fmt.Fprintf(&b, "  File: %s:%d\n", fx.File, fx.Line)
			if fx.Description != "" {
				fmt.Fprintf(&b, "  Action: %s\n", fx.Description)
			}
			if fx.Error != "" {
				fmt.Fprintf(&b, "  Error: %s\n", fx.Error)
			}
		}
	}

	manuals := collectManualReviewDecisions(rep.Decisions, findingsByID)
	if len(manuals) > 0 {
		fmt.Fprintf(&b, "\n-- Manual review required --\n")
		for _, item := range manuals {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}

	if len(rep.GeneratedTests) > 0 {
		fmt.Fprintf(&b, "\n-- Test generation --\n")

		tests := append([]types.TestGenerationResult(nil), rep.GeneratedTests...)
		sort.Slice(tests, func(i, j int) bool {
			return tests[i].Target < tests[j].Target
		})

		for _, tg := range tests {
			state := "SKIPPED"
			if tg.Generated {
				state = "GENERATED"
			} else if tg.Planned {
				state = "PLANNED"
			}

			fmt.Fprintf(&b, "- [%s] %s\n", state, tg.Target)
			if tg.Description != "" {
				fmt.Fprintf(&b, "  %s\n", tg.Description)
			}
			if tg.Error != "" {
				fmt.Fprintf(&b, "  Error: %s\n", tg.Error)
			}
		}
	}

	if len(rep.RemainingFindings) > 0 {
		fmt.Fprintf(&b, "\n-- Remaining findings --\n")

		remaining := append([]types.Finding(nil), rep.RemainingFindings...)
		sort.Slice(remaining, func(i, j int) bool {
			if remaining[i].Line == remaining[j].Line {
				return remaining[i].ID < remaining[j].ID
			}
			return remaining[i].Line < remaining[j].Line
		})

		for _, fd := range remaining {
			fmt.Fprintf(&b, "- [%s] %s (%s:%d)\n",
				fd.Priority,
				fd.Message,
				fd.File,
				fd.Line,
			)
		}
	}

	if hasLogs(rep) {
		fmt.Fprintf(&b, "\n-- Validation logs --\n")
		if rep.Validation.Logs.BuildLog != "" {
			fmt.Fprintf(&b, "Build log: %s\n", rep.Validation.Logs.BuildLog)
		}
		if rep.Validation.Logs.TestLog != "" {
			fmt.Fprintf(&b, "Test log: %s\n", rep.Validation.Logs.TestLog)
		}
		if rep.Validation.Logs.RaceLog != "" {
			fmt.Fprintf(&b, "Race log: %s\n", rep.Validation.Logs.RaceLog)
		}
		if rep.Validation.Logs.CoverageLog != "" {
			fmt.Fprintf(&b, "Coverage log: %s\n", rep.Validation.Logs.CoverageLog)
		}
	}

	return b.String()
}

func boolLabel(v bool) string {
	if v {
		return "PASS"
	}
	return "FAIL"
}

func fixStatus(fx types.FixResult) string {
	finalApplied := fx.Applied && fx.ValidationOK && !fx.RolledBack

	switch {
	case finalApplied:
		return "applied"
	case fx.RolledBack:
		return "rolled back"
	case fx.Planned:
		return "planned"
	case fx.Applied && !fx.ValidationOK:
		return "applied but failed validation"
	default:
		return "failed"
	}
}

func collectManualReviewDecisions(
	decisions []types.AgentDecision,
	findingsByID map[string]types.Finding,
) []string {
	var out []string

	for _, d := range decisions {
		if d.Kind != "finding" || d.Action != "manual_review" {
			continue
		}

		if fd, ok := findingsByID[d.TargetID]; ok {
			out = append(out, fmt.Sprintf(
				"%s (%s:%d, priority %s)",
				fd.Message,
				fd.File,
				fd.Line,
				fd.Priority,
			))
			continue
		}

		out = append(out, d.TargetID)
	}

	sort.Strings(out)
	return out
}

func hasLogs(rep types.Report) bool {
	return rep.Validation.Logs.BuildLog != "" ||
		rep.Validation.Logs.TestLog != "" ||
		rep.Validation.Logs.RaceLog != "" ||
		rep.Validation.Logs.CoverageLog != ""
}
