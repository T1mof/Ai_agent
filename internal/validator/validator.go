package validator

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ai_agent/internal/types"
)

func Run(projectRoot string, targetCoverage float64) types.ValidationResult {
	result := types.ValidationResult{
		CoverageGoal: targetCoverage,
		Logs: types.ValidationLogs{
			BuildLog:    filepath.Join(projectRoot, "build.log"),
			TestLog:     filepath.Join(projectRoot, "test.log"),
			RaceLog:     filepath.Join(projectRoot, "race.log"),
			CoverageLog: filepath.Join(projectRoot, "coverage.log"),
		},
		RaceStatus: "not_run",
	}

	result.BuildOK = runAndLog(projectRoot, result.Logs.BuildLog, "go", "build", "./...")
	result.TestOK = runAndLog(projectRoot, result.Logs.TestLog, "go", "test", "-count=1", "./...")

	raceOK, raceSupported := runRace(projectRoot, result.Logs.RaceLog)
	result.RaceOK = raceOK
	result.RaceSupported = raceSupported

	switch {
	case raceSupported && raceOK:
		result.RaceStatus = "passed"
	case raceSupported && !raceOK:
		result.RaceStatus = "failed"
	default:
		result.RaceStatus = "unsupported"
	}

	if result.TestOK {
		coverage, ok := runCoverage(projectRoot, result.Logs.CoverageLog)
		result.Coverage = coverage
		result.CoverageMet = ok && coverage >= targetCoverage
	}

	return result
}

func runAndLog(dir string, logPath string, name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	_ = os.WriteFile(logPath, out.Bytes(), 0o644)

	return err == nil
}

func runRace(dir string, logPath string) (bool, bool) {
	cmd := exec.Command("go", "test", "-count=1", "-race", "./...")
	cmd.Dir = dir

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	_ = os.WriteFile(logPath, out.Bytes(), 0o644)

	text := strings.ToLower(out.String())

	if err == nil {
		return true, true
	}

	if strings.Contains(text, "race is not supported") ||
		strings.Contains(text, "-race requires cgo") ||
		strings.Contains(text, "cgo is not enabled") ||
		strings.Contains(text, "requires gcc") ||
		strings.Contains(text, "requires mingw") ||
		strings.Contains(text, "not supported on") {
		return false, false
	}

	return false, true
}

func runCoverage(projectRoot string, logPath string) (float64, bool) {
	profilePath := filepath.Join(projectRoot, "coverage.out")

	cmd := exec.Command(
		"go",
		"test",
		"-count=1",
		"-covermode=count",
		"-coverpkg=./...",
		"-coverprofile=coverage.out",
		"./...",
	)
	cmd.Dir = projectRoot

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	logText := strings.Builder{}
	logText.WriteString("COMMAND: go test -count=1 -covermode=count -coverpkg=./... -coverprofile=coverage.out ./...\n\n")
	logText.Write(out.Bytes())

	if err != nil {
		logText.WriteString("\n\nERROR: coverage command failed\n")
		_ = os.WriteFile(logPath, []byte(logText.String()), 0o644)
		return 0, false
	}

	if _, statErr := os.Stat(profilePath); statErr != nil {
		logText.WriteString("\n\nERROR: coverage profile was not created\n")
		_ = os.WriteFile(logPath, []byte(logText.String()), 0o644)
		return 0, false
	}

	total, ok, parseNote := parseCoverageSummary(projectRoot, profilePath)
	if parseNote != "" {
		logText.WriteString("\n\n")
		logText.WriteString(parseNote)
	}

	if ok {
		htmlCmd := exec.Command("go", "tool", "cover", "-html=coverage.out", "-o", "coverage.html")
		htmlCmd.Dir = projectRoot
		var htmlOut bytes.Buffer
		htmlCmd.Stdout = &htmlOut
		htmlCmd.Stderr = &htmlOut
		_ = htmlCmd.Run()

		if htmlOut.Len() > 0 {
			logText.WriteString("\n\nHTML COVER OUTPUT:\n")
			logText.Write(htmlOut.Bytes())
		}
	}

	_ = os.WriteFile(logPath, []byte(logText.String()), 0o644)
	return total, ok
}

func parseCoverageSummary(projectRoot, profilePath string) (float64, bool, string) {
	toolCoverage, toolOK, toolRaw := parseCoverageWithTool(projectRoot, profilePath)
	if toolOK {
		return toolCoverage, true, "coverage parsed via go tool cover\n\n" + toolRaw
	}

	fallbackCoverage, fallbackOK, fallbackRaw := parseCoverageProfile(profilePath)
	if fallbackOK {
		return fallbackCoverage, true, "go tool cover parsing failed, fallback profile parsing used\n\n" + fallbackRaw
	}

	combined := "coverage parsing failed via go tool cover and fallback profile parsing"
	if toolRaw != "" {
		combined += "\n\nGO TOOL COVER OUTPUT:\n" + toolRaw
	}
	if fallbackRaw != "" {
		combined += "\n\nFALLBACK PARSER OUTPUT:\n" + fallbackRaw
	}

	return 0, false, combined
}

func parseCoverageWithTool(projectRoot, profilePath string) (float64, bool, string) {
	cmd := exec.Command("go", "tool", "cover", "-func="+filepath.Base(profilePath))
	cmd.Dir = projectRoot

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	raw := out.String()

	if err != nil {
		return 0, false, raw
	}

	re := regexp.MustCompile(`total:\s+\(statements\)\s+([0-9.]+)%`)
	sc := bufio.NewScanner(strings.NewReader(raw))

	for sc.Scan() {
		line := sc.Text()
		m := re.FindStringSubmatch(line)
		if len(m) == 2 {
			val, err := strconv.ParseFloat(m[1], 64)
			if err == nil {
				return val, true, raw
			}
		}
	}

	return 0, false, raw
}

func parseCoverageProfile(profilePath string) (float64, bool, string) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return 0, false, fmt.Sprintf("read profile error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return 0, false, "coverage profile has no data lines"
	}

	var totalStmts int64
	var coveredStmts int64

	for i, line := range lines[1:] {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}

		numStmts, err1 := strconv.ParseInt(fields[len(fields)-2], 10, 64)
		count, err2 := strconv.ParseInt(fields[len(fields)-1], 10, 64)
		if err1 != nil || err2 != nil {
			return 0, false, fmt.Sprintf("parse profile line %d failed: %s", i+2, line)
		}

		totalStmts += numStmts
		if count > 0 {
			coveredStmts += numStmts
		}
	}

	if totalStmts == 0 {
		return 0, false, "profile parsed but total statements is 0"
	}

	coverage := float64(coveredStmts) * 100.0 / float64(totalStmts)
	raw := fmt.Sprintf(
		"fallback parsed coverage profile\ncovered_statements=%d\ntotal_statements=%d\ncoverage=%.2f%%",
		coveredStmts,
		totalStmts,
		coverage,
	)

	return coverage, true, raw
}

func RollbackFromBackup(path string) error {
	backup := path + ".bak"

	data, err := os.ReadFile(backup)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func CleanupBackup(path string) error {
	backup := path + ".bak"

	err := os.Remove(backup)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}
