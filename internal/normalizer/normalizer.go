package normalizer

import (
	"sort"
	"strings"

	"ai_agent/internal/types"
)

func Normalize(in []types.Finding) []types.Finding {
	seen := make(map[string]bool)
	out := make([]types.Finding, 0, len(in))

	for _, f := range in {
		f.Category, f.Priority, f.FixCategory = classify(f)
		key := dedupKey(f)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			if out[i].File == out[j].File {
				return out[i].Line < out[j].Line
			}
			return out[i].File < out[j].File
		}
		return priorityWeight(out[i].Priority) < priorityWeight(out[j].Priority)
	})

	return out
}

func classify(f types.Finding) (category, priority, fixCategory string) {
	l := strings.ToLower(f.Message)

	switch {
	case strings.Contains(l, "nil"):
		return "nil_safety", "P2", "B"
	case strings.Contains(l, "close") || strings.Contains(l, "resource"):
		return "resource_management", "P1", "A"
	case strings.Contains(l, "error") || strings.Contains(l, "err"):
		return "error_handling", "P1", "A"
	default:
		return "general", "P3", "A"
	}
}

func dedupKey(f types.Finding) string {
	return f.Tool + "|" + f.File + "|" + f.Symbol + "|" + f.Message
}

func priorityWeight(p string) int {
	switch p {
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	default:
		return 4
	}
}
