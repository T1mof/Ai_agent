package fixer

import "ai_agent/internal/types"

type Fixer interface {
	Name() string
	CanApply(f types.Finding) bool
	Apply(projectRoot string, f types.Finding, functions []types.FunctionInfo) (types.FixResult, error)
}
