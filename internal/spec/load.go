package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"ai_agent/internal/types"
)

func Load(path string) (*types.Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	var s types.Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse spec yaml: %w", err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("spec.name is required")
	}
	if s.Language != "go" {
		return nil, fmt.Errorf("only language=go is supported")
	}
	if s.TargetCoverage <= 0 || s.TargetCoverage > 100 {
		return nil, fmt.Errorf("target_coverage must be in range (0,100]")
	}
	if len(s.Requirements) == 0 {
		return nil, fmt.Errorf("requirements must not be empty")
	}

	return &s, nil
}
