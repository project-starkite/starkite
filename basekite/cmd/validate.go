package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/lint"
	"github.com/spf13/cobra"
	"go.starlark.net/syntax"
)

var (
	validateJSON   bool
	validateLint   bool
	validateStrict bool
)

// SyntaxError represents a syntax error with location
type SyntaxError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

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
	Errors      []SyntaxError          `json:"errors,omitempty"`
	Diagnostics []ValidationDiagnostic `json:"diagnostics,omitempty"`
}

var validateCmd = &cobra.Command{
	Use:   "validate <script.star> [scripts...]",
	Short: "Validate script syntax and dialect conventions without executing",
	Long: `Validate one or more Starkite scripts for syntax errors and convention lints.

Checks performed:
  - Starlark grammar and syntax compliance
  - Dialect conventions (unchecked try_ Results, explicit top-level main() calls)
  - Deprecated functions and APIs (with modern alternatives)
  - Sandbox policy violation hints (unapproved subprocess execution, sensitive paths)
  - Starlark load() import existence and format verification

Exit codes:
  0 - All scripts are valid (and meet conventions when --strict is passed)
  1 - Syntax errors or lint violations in strict mode
  2 - File not found or read error

Examples:
  # Basic syntax validation
  kite validate deploy.star

  # Syntax plus convention and static analysis lints
  kite validate --lint deploy.star

  # Strict mode for CI/CD gates (fail on warnings)
  kite validate --lint --strict scripts/*.star

  # Machine-readable diagnostic output for AI agent self-correction
  kite validate --lint --json broken.star
`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output results as JSON")
	validateCmd.Flags().BoolVar(&validateLint, "lint", false, "Run dialect conventions and static analysis linter")
	validateCmd.Flags().BoolVar(&validateStrict, "strict", false, "Fail with exit code 1 if any lint warnings are detected")
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	var results []ValidationResult
	hasErrors := false
	hasWarnings := false
	hasFileErr := false

	for _, path := range args {
		result, isFileErr := validateScript(path)
		results = append(results, result)
		if !result.Valid {
			hasErrors = true
			if isFileErr {
				hasFileErr = true
			}
		}
		for _, d := range result.Diagnostics {
			if d.Severity == "error" {
				hasErrors = true
			} else if d.Severity == "warning" {
				hasWarnings = true
			}
		}
	}

	if validateJSON {
		// Output JSON
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results: %w", err)
		}
		fmt.Println(string(data))
	} else {
		// Human-readable output
		for _, result := range results {
			if result.Valid && len(result.Diagnostics) == 0 {
				fmt.Printf("%s: OK\n", result.File)
			} else {
				status := "OK (with lints)"
				if !result.Valid {
					status = fmt.Sprintf("FAILED (%d syntax errors)", len(result.Errors))
				}
				fmt.Printf("%s: %s\n", result.File, status)
				for _, err := range result.Errors {
					fmt.Printf("  %s:%d:%d: %s\n", err.File, err.Line, err.Column, err.Message)
				}
				for _, d := range result.Diagnostics {
					fmt.Printf("  %s:%d:%d: [%s %s] %s\n", d.File, d.Line, d.Column, d.Severity, d.Code, d.Message)
				}
			}
		}
	}

	if hasErrors || (validateStrict && hasWarnings) {
		exitCode := libkite.ExitScriptError
		if hasFileErr {
			exitCode = libkite.ExitFileError
		}
		return &libkite.ScriptError{
			Message:  "validation failed",
			ExitCode: exitCode,
		}
	}
	return nil
}

func validateScript(path string) (ValidationResult, bool) {
	result := ValidationResult{
		File:  path,
		Valid: true,
	}

	// Check if file exists
	content, err := os.ReadFile(path)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, SyntaxError{
			File:    path,
			Line:    0,
			Column:  0,
			Message: fmt.Sprintf("failed to read file: %v", err),
		})
		return result, true
	}

	// Parse the script
	fileOpt := &syntax.FileOptions{}
	parsedFile, err := fileOpt.Parse(path, content, 0)
	if err != nil {
		result.Valid = false
		result.Errors = parseStarlarkErrors(path, err)
		return result, false
	}

	if validateLint {
		callerDir := filepath.Dir(path)
		opts := lint.DefaultOptions(callerDir)
		findings := lint.Lint(parsedFile, path, opts)
		for _, f := range findings {
			result.Diagnostics = append(result.Diagnostics, ValidationDiagnostic{
				File:     f.File,
				Line:     f.Range.Start.Line,
				Column:   f.Range.Start.Column,
				EndLine:  f.Range.End.Line,
				EndCol:   f.Range.End.Column,
				Severity: f.Severity.String(),
				Code:     f.Code,
				Source:   f.Source,
				Message:  f.Message,
			})
		}
	}

	return result, false
}

var starlarkErrRe = regexp.MustCompile(`^(.+):(\d+):(\d+): (.+)$`)

// parseStarlarkErrors extracts location info from Starlark syntax errors
func parseStarlarkErrors(file string, err error) []SyntaxError {
	if err == nil {
		return nil
	}

	if synErr, ok := errors.AsType[syntax.Error](err); ok {
		fn := synErr.Pos.Filename()
		if fn == "" || fn == "<invalid>" {
			fn = file
		}
		return []SyntaxError{{
			File:    fn,
			Line:    int(synErr.Pos.Line),
			Column:  int(synErr.Pos.Col),
			Message: synErr.Msg,
		}}
	}

	// Starlark error format: "file:line:col: message"
	errStr := err.Error()

	// Try to parse location from error message
	if matches := starlarkErrRe.FindStringSubmatch(errStr); len(matches) == 5 {
		var line, col int
		fmt.Sscanf(matches[2], "%d", &line)
		fmt.Sscanf(matches[3], "%d", &col)
		return []SyntaxError{{
			File:    matches[1],
			Line:    line,
			Column:  col,
			Message: matches[4],
		}}
	}

	// Fallback: use error message as-is
	return []SyntaxError{{
		File:    file,
		Line:    1,
		Column:  1,
		Message: errStr,
	}}
}
