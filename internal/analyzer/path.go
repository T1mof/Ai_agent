package analyzer

import "path/filepath"

func normalizeProjectPath(projectRoot, path string) string {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return filepath.Clean(path)
	}

	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(absRoot, path)
	}

	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return filepath.Clean(absPath)
	}

	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return filepath.Clean(absPath)
	}

	return filepath.Clean(rel)
}
