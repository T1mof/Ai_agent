package agent

import (
	"path/filepath"
	"sort"
	"strings"

	"ai_agent/internal/types"
)

type Planner struct{}

type resolvedPolicy struct {
	minFixConfidence             float64
	minTestConfidence            float64
	minLLMConfidence             float64
	llmEnabled                   bool
	llmFallbackOnTemplateFailure bool

	enabledActions         map[string]bool
	enabledCategories      map[string]bool
	manualReviewCategories map[string]bool
	templateCategories     map[string]bool
	llmCategories          map[string]bool

	hasActionFilter   bool
	hasCategoryFilter bool
}

func NewPlanner() *Planner {
	return &Planner{}
}

func (p *Planner) BuildPlan(
	spec *types.Spec,
	findings []types.Finding,
	functions []types.FunctionInfo,
) []types.AgentDecision {
	policy := resolvePolicy(spec)

	var decisions []types.AgentDecision

	for _, f := range findings {
		d := p.planFinding(f)
		d = applyFindingPolicy(d, policy)
		decisions = append(decisions, d)
	}

	if spec != nil {
		for _, fn := range functions {
			if !isTargeted(spec.Targets, fn) {
				continue
			}
			d := p.planTestGeneration(fn)
			d = applyTestPolicy(d, policy)
			decisions = append(decisions, d)
		}
	}

	sort.SliceStable(decisions, func(i, j int) bool {
		pi := priorityRank(decisions[i].Priority)
		pj := priorityRank(decisions[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return decisions[i].Confidence > decisions[j].Confidence
	})

	return decisions
}

func DecisionIndex(decisions []types.AgentDecision) map[string]types.AgentDecision {
	out := make(map[string]types.AgentDecision, len(decisions))
	for _, d := range decisions {
		if d.Kind == "finding" {
			out[d.TargetID] = d
		}
	}
	return out
}

func ExecutableTestTargets(decisions []types.AgentDecision) []string {
	var out []string
	for _, d := range decisions {
		if d.Kind == "test_target" && d.Action == "generate_test" && d.Executable {
			out = append(out, d.TargetID)
		}
	}
	return out
}

func (p *Planner) planFinding(f types.Finding) types.AgentDecision {
	category := normalizedFindingCategory(f)

	return types.AgentDecision{
		Kind:        "finding",
		TargetID:    f.ID,
		Category:    category,
		Priority:    f.Priority,
		Confidence:  confidenceForFinding(f),
		Reason:      reasonForFinding(f),
		Description: descriptionForFinding(f),
	}
}

func (p *Planner) planTestGeneration(fn types.FunctionInfo) types.AgentDecision {
	conf := 0.80
	if isPointerTarget(fn) {
		conf = 0.78
	}
	if isPathTarget(fn) {
		conf = 0.86
	}

	return types.AgentDecision{
		Kind:        "test_target",
		TargetID:    normalizeTargetPath(fn.File) + ":" + fn.Name,
		Category:    "test_generation",
		Priority:    "P2",
		Confidence:  conf,
		Action:      "generate_test",
		Reason:      "function is explicitly targeted by spec and supported by generator",
		Description: "generate table-driven or scenario-based tests",
	}
}

func applyFindingPolicy(d types.AgentDecision, p resolvedPolicy) types.AgentDecision {
	if p.manualReviewCategories[d.Category] {
		d.Action = "manual_review"
		d.Status = "manual_review_required"
		d.Executable = false
		d.Reason += "; category is configured for manual review"
		return d
	}

	if p.hasCategoryFilter && !p.enabledCategories[d.Category] {
		d.Action = "skip"
		d.Status = "skipped_by_policy"
		d.Executable = false
		d.Reason += "; category is disabled by policy"
		return d
	}

	var plannedAction string

	switch {
	case p.templateCategories[d.Category]:
		plannedAction = "apply_template_fix"
	case p.llmCategories[d.Category]:
		if p.llmEnabled {
			plannedAction = "apply_llm_fix"
		} else {
			d.Action = "manual_review"
			d.Status = "manual_review_required"
			d.Executable = false
			d.Reason += "; llm category requires llm, but llm is disabled"
			return d
		}
	default:
		d.Action = "skip"
		d.Status = "skipped_by_capability"
		d.Executable = false
		d.Reason += "; no registered remediation strategy"
		return d
	}

	if p.hasActionFilter && !p.enabledActions[plannedAction] {
		d.Action = "skip"
		d.Status = "skipped_by_policy"
		d.Executable = false
		d.Reason += "; action is disabled by policy"
		return d
	}

	switch plannedAction {
	case "apply_template_fix":
		if d.Confidence < p.minFixConfidence {
			d.Action = "manual_review"
			d.Status = "manual_review_required"
			d.Executable = false
			d.Reason += "; confidence is below min_fix_confidence"
			return d
		}
	case "apply_llm_fix":
		if d.Confidence < p.minLLMConfidence {
			d.Action = "manual_review"
			d.Status = "manual_review_required"
			d.Executable = false
			d.Reason += "; confidence is below llm.min_confidence"
			return d
		}
	}

	d.Action = plannedAction
	d.Status = "approved"
	d.Executable = true
	return d
}

func applyTestPolicy(d types.AgentDecision, p resolvedPolicy) types.AgentDecision {
	if p.hasActionFilter && !p.enabledActions[d.Action] {
		d.Action = "skip"
		d.Status = "skipped_by_policy"
		d.Executable = false
		d.Reason += "; action is disabled by policy"
		return d
	}

	if d.Confidence < p.minTestConfidence {
		d.Action = "skip"
		d.Status = "skipped_by_policy"
		d.Executable = false
		d.Reason += "; confidence is below min_test_confidence"
		return d
	}

	d.Status = "approved"
	d.Executable = true
	return d
}

func resolvePolicy(spec *types.Spec) resolvedPolicy {
	r := resolvedPolicy{
		minFixConfidence:             0.80,
		minTestConfidence:            0.70,
		minLLMConfidence:             0.75,
		llmEnabled:                   false,
		llmFallbackOnTemplateFailure: false,
		enabledActions: map[string]bool{
			"apply_template_fix": true,
			"apply_llm_fix":      true,
			"generate_test":      true,
		},
		templateCategories: map[string]bool{
			"resource_management": true,
			"error_handling":      true,
			"nil_safety":          true,
			"ignored_error":       true,
		},
		llmCategories: map[string]bool{
			"concurrency": true,
		},
		manualReviewCategories: map[string]bool{},
	}

	if spec == nil {
		return r
	}

	r.llmEnabled = spec.LLM.Enabled
	if spec.LLM.MinConfidence > 0 {
		r.minLLMConfidence = spec.LLM.MinConfidence
	}
	r.llmFallbackOnTemplateFailure = spec.AgentPolicy.LLMFallbackOnTemplateFailure

	if spec.AgentPolicy.MinFixConfidence > 0 {
		r.minFixConfidence = spec.AgentPolicy.MinFixConfidence
	}
	if spec.AgentPolicy.MinTestConfidence > 0 {
		r.minTestConfidence = spec.AgentPolicy.MinTestConfidence
	}

	if len(spec.AgentPolicy.EnabledActions) > 0 {
		r.enabledActions = map[string]bool{}
		r.hasActionFilter = true
		for _, a := range spec.AgentPolicy.EnabledActions {
			r.enabledActions[a] = true
		}
	}

	if len(spec.AgentPolicy.EnabledCategories) > 0 {
		r.enabledCategories = map[string]bool{}
		r.hasCategoryFilter = true
		for _, c := range spec.AgentPolicy.EnabledCategories {
			r.enabledCategories[c] = true
		}
	}

	if len(spec.AgentPolicy.ManualReviewCategories) > 0 {
		r.manualReviewCategories = map[string]bool{}
		for _, c := range spec.AgentPolicy.ManualReviewCategories {
			r.manualReviewCategories[c] = true
		}
	}

	if len(spec.AgentPolicy.TemplateCategories) > 0 {
		r.templateCategories = map[string]bool{}
		for _, c := range spec.AgentPolicy.TemplateCategories {
			r.templateCategories[c] = true
		}
	}

	if len(spec.AgentPolicy.LLMCategories) > 0 {
		r.llmCategories = map[string]bool{}
		for _, c := range spec.AgentPolicy.LLMCategories {
			r.llmCategories[c] = true
		}
	}

	return r
}

func normalizedFindingCategory(f types.Finding) string {
	switch {
	case strings.HasPrefix(f.ID, "ast_ignored_error:"):
		return "ignored_error"
	default:
		return f.Category
	}
}

func reasonForFinding(f types.Finding) string {
	switch {
	case strings.HasPrefix(f.ID, "ast_ignored_error:"):
		return "pattern is covered by deterministic ignored-error template"
	case f.Category == "error_handling" && f.FixCategory == "A":
		return "pattern is covered by deterministic template return strategy"
	case f.Category == "resource_management" && f.FixCategory == "A":
		return "pattern is covered by deterministic defer Close template"
	case f.Category == "nil_safety" && f.FixCategory == "B":
		return "pattern is covered by deterministic nil-guard insertion"
	case f.Category == "concurrency":
		return "finding requires broader contextual reasoning and is better suited for llm or manual review"
	default:
		return "pattern requires a strategy decision from policy"
	}
}

func descriptionForFinding(f types.Finding) string {
	switch {
	case strings.HasPrefix(f.ID, "ast_ignored_error:"):
		return "safe automated template fix can be attempted for ignored error"
	case f.Category == "error_handling" && f.FixCategory == "A":
		return "safe automated template fix can be attempted"
	case f.Category == "resource_management" && f.FixCategory == "A":
		return "safe automated template fix can be attempted"
	case f.Category == "nil_safety" && f.FixCategory == "B":
		return "safe automated template fix can be attempted"
	case f.Category == "concurrency":
		return "prefer llm strategy or manual review"
	default:
		return "strategy will be chosen by policy"
	}
}

func confidenceForFinding(f types.Finding) float64 {
	switch {
	case strings.HasPrefix(f.ID, "ast_ignored_error:"):
		return 0.84
	case f.Category == "resource_management" && f.FixCategory == "A":
		return 0.95
	case f.Category == "error_handling" && f.FixCategory == "A":
		return 0.93
	case f.Category == "nil_safety" && f.FixCategory == "B":
		return 0.88
	case f.Category == "concurrency":
		return 0.72
	default:
		return 0.20
	}
}

func isTargeted(targets []string, fn types.FunctionInfo) bool {
	candidate := normalizeTargetPath(fn.File) + ":" + fn.Name

	for _, t := range targets {
		parts := strings.Split(t, ":")
		if len(parts) != 2 {
			continue
		}
		targetCandidate := normalizeTargetPath(parts[0]) + ":" + parts[1]
		if targetCandidate == candidate {
			return true
		}
	}

	return false
}

func normalizeTargetPath(p string) string {
	p = filepath.Clean(p)
	p = strings.ReplaceAll(p, "\\", "/")
	return p
}

func isPointerTarget(fn types.FunctionInfo) bool {
	for _, p := range fn.Params {
		if strings.HasPrefix(p.Type, "*") {
			return true
		}
	}
	return false
}

func isPathTarget(fn types.FunctionInfo) bool {
	if len(fn.Params) != 1 {
		return false
	}
	if fn.Params[0].Type != "string" {
		return false
	}
	name := strings.ToLower(fn.Params[0].Name)
	return strings.Contains(name, "path") || strings.Contains(name, "file")
}

func priorityRank(p string) int {
	switch p {
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	case "P4":
		return 4
	default:
		return 9
	}
}
