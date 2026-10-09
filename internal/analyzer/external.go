package analyzer

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ai_agent/internal/types"
)

type ExternalAnalyzer struct {
	tool string
	args []string
}

func NewGoVetAnalyzer() Analyzer {
	return &ExternalAnalyzer{
		tool: "go",
		args: []string{"vet", "./..."},
	}
}

func NewStaticcheckAnalyzer() Analyzer {
	return &ExternalAnalyzer{
		tool: "staticcheck",
		args: []string{"./..."},
	}
}

func (a *ExternalAnalyzer) Name() string {
	return a.tool
}

func (a *ExternalAnalyzer) Run(projectRoot string) ([]types.Finding, error) {
	cmd := exec.Command(a.tool, a.args...)
	cmd.Dir = projectRoot

	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr

	_ = cmd.Run()

	return parseExternalOutput(a.tool, stderr.String(), projectRoot), nil
}

var diagRe = regexp.MustCompile(`^(.+\.go):(\d+):(?:(\d+):)?\s*(.+)$`)

func parseExternalOutput(tool, output, projectRoot string) []types.Finding {
	var out []types.Finding
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		m := diagRe.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}

		file := normalizeProjectPath(projectRoot, m[1])
		lineNum, _ := strconv.Atoi(m[2])
		msg := strings.TrimSpace(m[4])

		category, priority, fixCategory := classify(tool, msg)

		out = append(out, types.Finding{
			ID:          tool + ":" + filepath.Base(file) + ":" + strconv.Itoa(lineNum),
			Tool:        tool,
			File:        file,
			Line:        lineNum,
			Category:    category,
			Priority:    priority,
			FixCategory: fixCategory,
			Message:     msg,
		})
	}

	return out
}

func classify(tool, msg string) (category, priority, fixCategory string) {
	l := strings.ToLower(msg)

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
