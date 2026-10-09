package types

type Spec struct {
	Name           string      `yaml:"name" json:"name"`
	Language       string      `yaml:"language" json:"language"`
	TargetCoverage float64     `yaml:"target_coverage" json:"target_coverage"`
	Requirements   []string    `yaml:"requirements" json:"requirements"`
	Targets        []string    `yaml:"targets,omitempty" json:"targets,omitempty"`
	TestScenarios  []string    `yaml:"test_scenarios,omitempty" json:"test_scenarios,omitempty"`
	AgentPolicy    AgentPolicy `yaml:"agent_policy,omitempty" json:"agent_policy,omitempty"`
	LLM            LLMConfig   `yaml:"llm,omitempty" json:"llm,omitempty"`
}

type LLMConfig struct {
	Enabled         bool    `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Provider        string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	BaseURL         string  `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	Model           string  `yaml:"model,omitempty" json:"model,omitempty"`
	Mode            string  `yaml:"mode,omitempty" json:"mode,omitempty"`
	MinConfidence   float64 `yaml:"min_confidence,omitempty" json:"min_confidence,omitempty"`
	LocalConfigPath string  `yaml:"local_config_path,omitempty" json:"local_config_path,omitempty"`
}

type AgentPolicy struct {
	MinFixConfidence             float64  `yaml:"min_fix_confidence,omitempty" json:"min_fix_confidence,omitempty"`
	MinTestConfidence            float64  `yaml:"min_test_confidence,omitempty" json:"min_test_confidence,omitempty"`
	EnabledActions               []string `yaml:"enabled_actions,omitempty" json:"enabled_actions,omitempty"`
	EnabledCategories            []string `yaml:"enabled_categories,omitempty" json:"enabled_categories,omitempty"`
	ManualReviewCategories       []string `yaml:"manual_review_categories,omitempty" json:"manual_review_categories,omitempty"`
	TemplateCategories           []string `yaml:"template_categories,omitempty" json:"template_categories,omitempty"`
	LLMCategories                []string `yaml:"llm_categories,omitempty" json:"llm_categories,omitempty"`
	LLMFallbackOnTemplateFailure bool     `yaml:"llm_fallback_on_template_failure,omitempty" json:"llm_fallback_on_template_failure,omitempty"`
}

type ParamInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type FunctionInfo struct {
	File       string      `json:"file"`
	Package    string      `json:"package"`
	Name       string      `json:"name"`
	Receiver   string      `json:"receiver,omitempty"`
	Line       int         `json:"line"`
	ReturnsErr bool        `json:"returns_err"`
	Results    []string    `json:"results,omitempty"`
	Params     []ParamInfo `json:"params,omitempty"`
}

type Finding struct {
	ID          string `json:"id"`
	Tool        string `json:"tool"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Category    string `json:"category"`
	Priority    string `json:"priority"`
	FixCategory string `json:"fix_category"`
	Message     string `json:"message"`
	Symbol      string `json:"symbol,omitempty"`
}

type FixResult struct {
	FindingID     string `json:"finding_id"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Strategy      string `json:"strategy"`
	Description   string `json:"description,omitempty"`
	BeforeSnippet string `json:"before_snippet,omitempty"`
	AfterSnippet  string `json:"after_snippet,omitempty"`
	Planned       bool   `json:"planned,omitempty"`
	Applied       bool   `json:"applied"`
	RolledBack    bool   `json:"rolled_back"`
	ValidationOK  bool   `json:"validation_ok"`
	Error         string `json:"error,omitempty"`
}

type TestGenerationResult struct {
	Target      string `json:"target"`
	File        string `json:"file"`
	Generated   bool   `json:"generated"`
	Planned     bool   `json:"planned,omitempty"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
}

type ValidationLogs struct {
	BuildLog    string `json:"build_log,omitempty"`
	TestLog     string `json:"test_log,omitempty"`
	RaceLog     string `json:"race_log,omitempty"`
	CoverageLog string `json:"coverage_log,omitempty"`
}

type ValidationResult struct {
	BuildOK       bool           `json:"build_ok"`
	TestOK        bool           `json:"test_ok"`
	RaceOK        bool           `json:"race_ok"`
	RaceSupported bool           `json:"race_supported"`
	RaceStatus    string         `json:"race_status,omitempty"`
	Coverage      float64        `json:"coverage"`
	CoverageGoal  float64        `json:"coverage_goal"`
	CoverageMet   bool           `json:"coverage_met"`
	Logs          ValidationLogs `json:"logs,omitempty"`
}

type AgentDecision struct {
	Kind        string  `json:"kind"`
	TargetID    string  `json:"target_id"`
	Category    string  `json:"category,omitempty"`
	Priority    string  `json:"priority"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason"`
	Action      string  `json:"action"`
	Status      string  `json:"status,omitempty"`
	Executable  bool    `json:"executable"`
	Description string  `json:"description,omitempty"`
}

type SummaryMetrics struct {
	FindingsTotal     int `json:"findings_total"`
	RemainingFindings int `json:"remaining_findings"`
	FindingsResolved  int `json:"findings_resolved"`

	FixesAttempted  int `json:"fixes_attempted"`
	FixesApplied    int `json:"fixes_applied"`
	FixesRolledBack int `json:"fixes_rolled_back"`
	PlannedFixes    int `json:"planned_fixes"`

	TemplateApplied   int `json:"template_applied"`
	LLMAttempted      int `json:"llm_attempted"`
	LLMApplied        int `json:"llm_applied"`
	LLMFailed         int `json:"llm_failed"`
	ManualReviewCount int `json:"manual_review_count"`

	TestsGenerated int `json:"tests_generated"`
	PlannedTests   int `json:"planned_tests"`
	SkippedTests   int `json:"skipped_tests"`

	CoverageBefore float64 `json:"coverage_before"`
	CoverageAfter  float64 `json:"coverage_after"`
	CoverageDelta  float64 `json:"coverage_delta"`
}

type Report struct {
	Project            string                 `json:"project"`
	Spec               *Spec                  `json:"spec,omitempty"`
	Functions          []FunctionInfo         `json:"functions"`
	Findings           []Finding              `json:"findings"`
	RemainingFindings  []Finding              `json:"remaining_findings,omitempty"`
	Decisions          []AgentDecision        `json:"decisions,omitempty"`
	Fixes              []FixResult            `json:"fixes,omitempty"`
	GeneratedTests     []TestGenerationResult `json:"generated_tests,omitempty"`
	BaselineValidation ValidationResult       `json:"baseline_validation"`
	Validation         ValidationResult       `json:"validation"`
	Summary            SummaryMetrics         `json:"summary"`
}

type CommandResult struct {
	Command string `json:"command"`
	Report  Report `json:"report"`
}
