package analyzer

import "ai_agent/internal/types"

type Analyzer interface {
	Name() string
	Run(projectRoot string) ([]types.Finding, error)
}

func RunAll(projectRoot string, analyzers ...Analyzer) ([]types.Finding, error) {
	var all []types.Finding

	for _, a := range analyzers {
		findings, err := a.Run(projectRoot)
		if err != nil {
			return all, err
		}
		all = append(all, findings...)
	}

	return deduplicate(all), nil
}

func deduplicate(in []types.Finding) []types.Finding {
	seen := make(map[string]bool)
	out := make([]types.Finding, 0, len(in))

	for _, f := range in {
		key := f.Tool + "|" + f.File + "|" + f.Category + "|" + f.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}

	return out
}
