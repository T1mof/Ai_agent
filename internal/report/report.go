package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai_agent/internal/types"
)

func Write(path string, r types.Report, format string, mode string) error {
	format = normalizeFormat(format)
	mode = normalizeMode(mode)

	if format == "json" || format == "both" {
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}

	if format == "md" || format == "both" {
		mdPath := path
		if filepath.Ext(mdPath) != ".md" {
			mdPath = strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
		}

		content := buildCompactMarkdownReport(r)
		if mode == "full" {
			content = buildFullMarkdownReport(r)
		}

		if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}

func normalizeFormat(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "md":
		return "md"
	case "both":
		return "both"
	default:
		return "json"
	}
}

func normalizeMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "full":
		return "full"
	default:
		return "compact"
	}
}

func buildCompactMarkdownReport(r types.Report) string {
	var b strings.Builder

	b.WriteString("# Ai_agent Report\n\n")
	b.WriteString(fmt.Sprintf("**Project:** `%s`\n\n", r.Project))

	b.WriteString("## Highlights\n\n")
	b.WriteString(fmt.Sprintf("- **Findings detected:** %d\n", r.Summary.FindingsTotal))
	b.WriteString(fmt.Sprintf("- **Findings resolved:** %d\n", r.Summary.FindingsResolved))
	b.WriteString(fmt.Sprintf("- **Remaining findings:** %d\n", r.Summary.RemainingFindings))
	b.WriteString(fmt.Sprintf("- **Fixes applied:** %d\n", r.Summary.FixesApplied))
	b.WriteString(fmt.Sprintf("- **Tests generated:** %d\n", r.Summary.TestsGenerated))
	b.WriteString(fmt.Sprintf("- **Coverage before:** %.2f%%\n", r.Summary.CoverageBefore))
	b.WriteString(fmt.Sprintf("- **Coverage after:** %.2f%%\n", r.Summary.CoverageAfter))
	b.WriteString(fmt.Sprintf("- **Coverage delta:** %.2f%%\n", r.Summary.CoverageDelta))
	b.WriteString(fmt.Sprintf("- **Coverage goal met:** %v\n", r.Validation.CoverageMet))
	b.WriteString(fmt.Sprintf("- **Build OK:** %v\n", r.Validation.BuildOK))
	b.WriteString(fmt.Sprintf("- **Test OK:** %v\n", r.Validation.TestOK))
	b.WriteString(fmt.Sprintf("- **Race status:** `%s`\n\n", safeRaceStatus(r.Validation.RaceStatus)))

	if len(r.Decisions) > 0 {
		b.WriteString("## Key agent decisions\n\n")
		maxItems := len(r.Decisions)
		if maxItems > 6 {
			maxItems = 6
		}
		for i := 0; i < maxItems; i++ {
			d := r.Decisions[i]
			b.WriteString(fmt.Sprintf("- [`%s`, %.2f] **%s** → `%s`\n",
				d.Priority, d.Confidence, d.Action, d.TargetID))
		}
		b.WriteString("\n")
	}

	if len(r.GeneratedTests) > 0 {
		b.WriteString("## Generated tests\n\n")
		for _, tg := range r.GeneratedTests {
			status := "planned"
			if tg.Generated {
				status = "generated"
			}
			b.WriteString(fmt.Sprintf("- **%s** — `%s`\n", status, tg.Target))
		}
		b.WriteString("\n")
	}

	if len(r.Validation.Logs.BuildLog) > 0 || len(r.Validation.Logs.TestLog) > 0 || len(r.Validation.Logs.RaceLog) > 0 || len(r.Validation.Logs.CoverageLog) > 0 {
		b.WriteString("## Validation artifacts\n\n")
		if r.Validation.Logs.BuildLog != "" {
			b.WriteString(fmt.Sprintf("- **Build log:** `%s`\n", r.Validation.Logs.BuildLog))
		}
		if r.Validation.Logs.TestLog != "" {
			b.WriteString(fmt.Sprintf("- **Test log:** `%s`\n", r.Validation.Logs.TestLog))
		}
		if r.Validation.Logs.RaceLog != "" {
			b.WriteString(fmt.Sprintf("- **Race log:** `%s`\n", r.Validation.Logs.RaceLog))
		}
		if r.Validation.Logs.CoverageLog != "" {
			b.WriteString(fmt.Sprintf("- **Coverage log:** `%s`\n", r.Validation.Logs.CoverageLog))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Outcome\n\n")
	switch {
	case isSuccessfulOutcome(r):
		b.WriteString("Prototype run is successful: findings were resolved, tests were generated, and the coverage goal was achieved.\n")
		if !r.Validation.RaceSupported {
			b.WriteString("Race detector was unavailable in the current environment, so race validation is marked as unsupported.\n")
		}
	case isDryRun(r):
		b.WriteString("Dry-run completed successfully: the agent built an execution plan without modifying the project.\n")
	default:
		b.WriteString("Prototype run is only partially successful: inspect remaining findings, validation state, and decisions.\n")
	}

	return b.String()
}

func buildFullMarkdownReport(r types.Report) string {
	var b strings.Builder

	b.WriteString("# Ai_agent Report\n\n")
	b.WriteString(fmt.Sprintf("**Project:** `%s`\n\n", r.Project))

	if r.Spec != nil {
		b.WriteString("## Spec\n\n")
		b.WriteString(fmt.Sprintf("- **Name:** `%s`\n", r.Spec.Name))
		b.WriteString(fmt.Sprintf("- **Language:** `%s`\n", r.Spec.Language))
		b.WriteString(fmt.Sprintf("- **Target coverage:** `%.2f%%`\n", r.Spec.TargetCoverage))

		if len(r.Spec.Requirements) > 0 {
			b.WriteString("\n### Requirements\n\n")
			for _, req := range r.Spec.Requirements {
				b.WriteString(fmt.Sprintf("- %s\n", req))
			}
		}

		if len(r.Spec.Targets) > 0 {
			b.WriteString("\n### Targets\n\n")
			for _, target := range r.Spec.Targets {
				b.WriteString(fmt.Sprintf("- `%s`\n", target))
			}
		}

		if len(r.Spec.TestScenarios) > 0 {
			b.WriteString("\n### Test scenarios\n\n")
			for _, scenario := range r.Spec.TestScenarios {
				b.WriteString(fmt.Sprintf("- %s\n", scenario))
			}
		}

		if r.Spec.AgentPolicy.MinFixConfidence > 0 || r.Spec.AgentPolicy.MinTestConfidence > 0 || len(r.Spec.AgentPolicy.EnabledActions) > 0 || len(r.Spec.AgentPolicy.EnabledCategories) > 0 || len(r.Spec.AgentPolicy.ManualReviewCategories) > 0 {
			b.WriteString("\n### Agent policy\n\n")
			if r.Spec.AgentPolicy.MinFixConfidence > 0 {
				b.WriteString(fmt.Sprintf("- **Min fix confidence:** %.2f\n", r.Spec.AgentPolicy.MinFixConfidence))
			}
			if r.Spec.AgentPolicy.MinTestConfidence > 0 {
				b.WriteString(fmt.Sprintf("- **Min test confidence:** %.2f\n", r.Spec.AgentPolicy.MinTestConfidence))
			}
			for _, a := range r.Spec.AgentPolicy.EnabledActions {
				b.WriteString(fmt.Sprintf("- **Enabled action:** `%s`\n", a))
			}
			for _, c := range r.Spec.AgentPolicy.EnabledCategories {
				b.WriteString(fmt.Sprintf("- **Enabled category:** `%s`\n", c))
			}
			for _, c := range r.Spec.AgentPolicy.ManualReviewCategories {
				b.WriteString(fmt.Sprintf("- **Manual review category:** `%s`\n", c))
			}
		}

		b.WriteString("\n")
	}

	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Findings total:** %d\n", r.Summary.FindingsTotal))
	b.WriteString(fmt.Sprintf("- **Remaining findings:** %d\n", r.Summary.RemainingFindings))
	b.WriteString(fmt.Sprintf("- **Findings resolved:** %d\n", r.Summary.FindingsResolved))
	b.WriteString(fmt.Sprintf("- **Fixes attempted:** %d\n", r.Summary.FixesAttempted))
	b.WriteString(fmt.Sprintf("- **Fixes applied:** %d\n", r.Summary.FixesApplied))
	b.WriteString(fmt.Sprintf("- **Fixes rolled back:** %d\n", r.Summary.FixesRolledBack))
	b.WriteString(fmt.Sprintf("- **Planned fixes:** %d\n", r.Summary.PlannedFixes))
	b.WriteString(fmt.Sprintf("- **Tests generated:** %d\n", r.Summary.TestsGenerated))
	b.WriteString(fmt.Sprintf("- **Planned tests:** %d\n", r.Summary.PlannedTests))
	b.WriteString(fmt.Sprintf("- **Coverage before:** %.2f%%\n", r.Summary.CoverageBefore))
	b.WriteString(fmt.Sprintf("- **Coverage after:** %.2f%%\n", r.Summary.CoverageAfter))
	b.WriteString(fmt.Sprintf("- **Coverage delta:** %.2f%%\n\n", r.Summary.CoverageDelta))

	b.WriteString("## Validation\n\n")
	b.WriteString("### Baseline\n\n")
	b.WriteString(fmt.Sprintf("- **Build OK:** %v\n", r.BaselineValidation.BuildOK))
	b.WriteString(fmt.Sprintf("- **Test OK:** %v\n", r.BaselineValidation.TestOK))
	b.WriteString(fmt.Sprintf("- **Race status:** `%s`\n", safeRaceStatus(r.BaselineValidation.RaceStatus)))
	b.WriteString(fmt.Sprintf("- **Coverage:** %.2f%%\n", r.BaselineValidation.Coverage))
	b.WriteString(fmt.Sprintf("- **Coverage goal:** %.2f%%\n", r.BaselineValidation.CoverageGoal))
	b.WriteString(fmt.Sprintf("- **Coverage met:** %v\n\n", r.BaselineValidation.CoverageMet))

	b.WriteString("### Final\n\n")
	b.WriteString(fmt.Sprintf("- **Build OK:** %v\n", r.Validation.BuildOK))
	b.WriteString(fmt.Sprintf("- **Test OK:** %v\n", r.Validation.TestOK))
	b.WriteString(fmt.Sprintf("- **Race status:** `%s`\n", safeRaceStatus(r.Validation.RaceStatus)))
	b.WriteString(fmt.Sprintf("- **Coverage:** %.2f%%\n", r.Validation.Coverage))
	b.WriteString(fmt.Sprintf("- **Coverage goal:** %.2f%%\n", r.Validation.CoverageGoal))
	b.WriteString(fmt.Sprintf("- **Coverage met:** %v\n\n", r.Validation.CoverageMet))

	if len(r.Decisions) > 0 {
		b.WriteString("## Agent decisions\n\n")
		for i, d := range r.Decisions {
			b.WriteString(fmt.Sprintf("### %d. `%s`\n\n", i+1, d.TargetID))
			b.WriteString(fmt.Sprintf("- **Kind:** `%s`\n", d.Kind))
			if d.Category != "" {
				b.WriteString(fmt.Sprintf("- **Category:** `%s`\n", d.Category))
			}
			b.WriteString(fmt.Sprintf("- **Priority:** `%s`\n", d.Priority))
			b.WriteString(fmt.Sprintf("- **Action:** `%s`\n", d.Action))
			b.WriteString(fmt.Sprintf("- **Status:** `%s`\n", d.Status))
			b.WriteString(fmt.Sprintf("- **Executable:** %v\n", d.Executable))
			b.WriteString(fmt.Sprintf("- **Confidence:** %.2f\n", d.Confidence))
			b.WriteString(fmt.Sprintf("- **Reason:** %s\n", d.Reason))
			if d.Description != "" {
				b.WriteString(fmt.Sprintf("- **Description:** %s\n", d.Description))
			}
			b.WriteString("\n")
		}
	}

	if len(r.Findings) > 0 {
		b.WriteString("## Findings\n\n")
		for i, f := range r.Findings {
			b.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, f.Message))
			b.WriteString(fmt.Sprintf("- **ID:** `%s`\n", f.ID))
			b.WriteString(fmt.Sprintf("- **Tool:** `%s`\n", f.Tool))
			b.WriteString(fmt.Sprintf("- **File:** `%s`\n", f.File))
			b.WriteString(fmt.Sprintf("- **Line:** %d\n", f.Line))
			b.WriteString(fmt.Sprintf("- **Category:** `%s`\n", f.Category))
			b.WriteString(fmt.Sprintf("- **Priority:** `%s`\n", f.Priority))
			b.WriteString(fmt.Sprintf("- **Fix category:** `%s`\n", f.FixCategory))
			if f.Symbol != "" {
				b.WriteString(fmt.Sprintf("- **Symbol:** `%s`\n", f.Symbol))
			}
			b.WriteString("\n")
		}
	}

	if len(r.Fixes) > 0 {
		b.WriteString("## Fixes\n\n")
		for i, fx := range r.Fixes {
			b.WriteString(fmt.Sprintf("### %d. `%s`\n\n", i+1, fx.FindingID))
			b.WriteString(fmt.Sprintf("- **File:** `%s`\n", fx.File))
			b.WriteString(fmt.Sprintf("- **Line:** %d\n", fx.Line))
			b.WriteString(fmt.Sprintf("- **Strategy:** `%s`\n", fx.Strategy))
			if fx.Description != "" {
				b.WriteString(fmt.Sprintf("- **Description:** %s\n", fx.Description))
			}
			b.WriteString(fmt.Sprintf("- **Planned:** %v\n", fx.Planned))
			b.WriteString(fmt.Sprintf("- **Applied:** %v\n", fx.Applied))
			b.WriteString(fmt.Sprintf("- **Rolled back:** %v\n", fx.RolledBack))
			b.WriteString(fmt.Sprintf("- **Validation OK:** %v\n", fx.ValidationOK))
			if fx.Error != "" {
				b.WriteString(fmt.Sprintf("- **Error:** %s\n", fx.Error))
			}
			if fx.BeforeSnippet != "" {
				b.WriteString("\n**Before**\n\n```go\n")
				b.WriteString(fx.BeforeSnippet)
				b.WriteString("\n```\n")
			}
			if fx.AfterSnippet != "" {
				b.WriteString("\n**After**\n\n```go\n")
				b.WriteString(fx.AfterSnippet)
				b.WriteString("\n```\n")
			}
			b.WriteString("\n")
		}
	}

	if len(r.GeneratedTests) > 0 {
		b.WriteString("## Generated tests\n\n")
		for i, tg := range r.GeneratedTests {
			b.WriteString(fmt.Sprintf("### %d. `%s`\n\n", i+1, tg.Target))
			b.WriteString(fmt.Sprintf("- **File:** `%s`\n", tg.File))
			b.WriteString(fmt.Sprintf("- **Planned:** %v\n", tg.Planned))
			b.WriteString(fmt.Sprintf("- **Generated:** %v\n", tg.Generated))
			if tg.Description != "" {
				b.WriteString(fmt.Sprintf("- **Description:** %s\n", tg.Description))
			}
			if tg.Error != "" {
				b.WriteString(fmt.Sprintf("- **Error:** %s\n", tg.Error))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

func safeRaceStatus(s string) string {
	if strings.TrimSpace(s) == "" {
		return "not_run"
	}
	return s
}

func isSuccessfulOutcome(r types.Report) bool {
	if !r.Validation.BuildOK || !r.Validation.TestOK {
		return false
	}
	if !r.Validation.CoverageMet {
		return false
	}
	if r.Validation.RaceSupported && !r.Validation.RaceOK {
		return false
	}
	if r.Summary.RemainingFindings > 0 {
		return false
	}
	return true
}

func isDryRun(r types.Report) bool {
	return r.Summary.FixesApplied == 0 &&
		r.Summary.TestsGenerated == 0 &&
		r.Summary.PlannedFixes > 0
}
