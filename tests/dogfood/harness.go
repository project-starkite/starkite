package dogfood

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// ValidationDiagnostic represents a structured lint finding for CLI and JSON output.
type ValidationDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	EndLine  int    `json:"end_line,omitempty"`
	EndCol   int    `json:"end_column,omitempty"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

// ValidationResult represents the comprehensive validation result of a script.
type ValidationResult struct {
	File        string                 `json:"file"`
	Valid       bool                   `json:"valid"`
	Diagnostics []ValidationDiagnostic `json:"diagnostics,omitempty"`
}

// RunValidate executes `kite validate --lint --json` on a script file and parses JSON from stdout.
func RunValidate(kiteBin, scriptPath string) ([]ValidationResult, error) {
	cmd := exec.Command(kiteBin, "validate", "--lint", "--json", scriptPath)
	stdout, err := cmd.Output()
	// Even if validate returns non-zero exit code on validation errors,
	// stdout contains the structured JSON diagnostic output.
	if len(stdout) == 0 && err != nil {
		return nil, fmt.Errorf("failed to run validate: %w", err)
	}

	var results []ValidationResult
	if err := json.Unmarshal(stdout, &results); err != nil {
		return nil, fmt.Errorf("failed to parse validate JSON output: %w\nOutput: %s", err, string(stdout))
	}
	return results, nil
}

// RunScript executes `kite run` on a script file with optional sandbox allowance arguments.
func RunScript(kiteBin, scriptPath string, extraArgs ...string) (string, error) {
	args := append([]string{"run", scriptPath}, extraArgs...)
	cmd := exec.Command(kiteBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kite run failed (%w): %s", err, string(out))
	}
	return string(out), nil
}

// HasDiagnosticCode reports whether any finding in results contains the specified code.
func HasDiagnosticCode(results []ValidationResult, code string) bool {
	for _, res := range results {
		for _, d := range res.Diagnostics {
			if d.Code == code {
				return true
			}
		}
	}
	return false
}

// HasSeverity reports whether any finding in results matches the specified severity.
func HasSeverity(results []ValidationResult, severity string) bool {
	for _, res := range results {
		for _, d := range res.Diagnostics {
			if d.Severity == severity {
				return true
			}
		}
	}
	return false
}
