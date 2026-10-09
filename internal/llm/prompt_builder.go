package llm

import (
	"fmt"
	"strings"
)

func BuildPatchPrompt(req PatchRequest) (string, string) {
	system := `You are a Go code remediation assistant.
Generate the smallest possible compile-safe patch for exactly one Go function.

Hard rules:
1. Keep the original function signature unchanged.
2. Change only the target function.
3. Do not rename the function.
4. Do not modify imports.
5. Do not modify any other functions.
6. Do not add comments or explanations inside code.
7. Preserve existing behavior except for the minimum change required to address the finding.
8. Return STRICT JSON only with keys:
   - updated_function
   - confidence
   - rationale
9. updated_function must contain the COMPLETE updated Go function source only.
10. Do not use markdown fences.
11. confidence must be a number from 0.0 to 1.0.

Category-specific guidance:
- ignored_error:
  Prefer a minimal fix that captures or returns the error directly.
- error_handling:
  Preserve return arity exactly.
  If the function returns multiple values and one of them is error, then on error paths return the appropriate zero values plus err.
  Do not silently swallow the error.
- resource_management:
  Prefer minimal safe resource cleanup such as defer Close after successful open.
- nil_safety:
  Prefer a minimal guard at function entry without changing normal success behavior.

If unsure, make the smallest possible safe patch.`

	user := fmt.Sprintf(
		`Fix the following Go function.

Finding:
- ID: %s
- Category: %s
- Message: %s
- File: %s
- Line: %d

Target function name:
%s

Current function source:
%s

Surrounding source (read-only context, do not rewrite other functions):
%s

Important reminder:
- Output only one updated Go function.
- Keep the exact same function name and signature.
- Do not change imports.
- Do not modify neighboring functions.
- For error_handling findings in multi-return functions, return zero values plus err on failing paths.

Return STRICT JSON only.`,
		req.Finding.ID,
		normalizedCategory(req.Finding.Category),
		req.Finding.Message,
		req.FilePath,
		req.Finding.Line,
		req.FunctionName,
		req.FunctionSource,
		req.SurroundingSource,
	)

	return system, user
}

func normalizedCategory(category string) string {
	category = strings.TrimSpace(strings.ToLower(category))
	if category == "" {
		return "unknown"
	}
	return category
}
