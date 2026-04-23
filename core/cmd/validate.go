package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"go.starlark.net/syntax"
)

var (
	validateJSON bool
)

// SyntaxError represents a syntax error with location
type SyntaxError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// ValidationResult represents the result of validating a single file
type ValidationResult struct {
	File   string        `json:"file"`
	Valid  bool          `json:"valid"`
	Errors []SyntaxError `json:"errors,omitempty"`
}

var validateCmd = &cobra.Command{
	Use:   "validate <script.star> [scripts...]",
	Short: "Validate script syntax without executing",
	Long: `Validate one or more starkite scripts for syntax errors without executing them.

This is useful for CI/CD pipelines, pre-commit hooks, and editor integration.

Exit codes:
  0 - All scripts are valid
  1 - One or more scripts have errors
  2 - File not found or read error

Examples:
  # Validate a single script
  kite validate deploy.star

  # Validate multiple scripts
  kite validate *.star

  # Machine-readable output
  kite validate --json broken.star
`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output results as JSON")
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	var results []ValidationResult
	hasErrors := false

	for _, path := range args {
		result := validateScript(path)
		results = append(results, result)
		if !result.Valid {
			hasErrors = true
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
			if result.Valid {
				fmt.Printf("%s: OK\n", result.File)
			} else {
				fmt.Printf("%s: FAILED (%d errors)\n", result.File, len(result.Errors))
				for _, err := range result.Errors {
					fmt.Printf("  %s:%d:%d: %s\n", err.File, err.Line, err.Column, err.Message)
				}
			}
		}
	}

	if hasErrors {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func validateScript(path string) ValidationResult {
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
		return result
	}

	// Parse the script (syntax check only)
	_, err = syntax.Parse(path, content, 0)
	if err != nil {
		result.Valid = false
		result.Errors = parseStarlarkErrors(path, err)
	}

	return result
}

// parseStarlarkErrors extracts location info from Starlark syntax errors
func parseStarlarkErrors(file string, err error) []SyntaxError {
	var errors []SyntaxError

	// Starlark error format: "file:line:col: message"
	errStr := err.Error()
	
	// Try to parse location from error message
	// Pattern: "filename:line:col: message"
	re := regexp.MustCompile(`^([^:]+):(\d+):(\d+): (.+)$`)
	matches := re.FindStringSubmatch(errStr)
	
	if len(matches) == 5 {
		var line, col int
		fmt.Sscanf(matches[2], "%d", &line)
		fmt.Sscanf(matches[3], "%d", &col)
		errors = append(errors, SyntaxError{
			File:    matches[1],
			Line:    line,
			Column:  col,
			Message: matches[4],
		})
	} else {
		// Fallback: use error message as-is
		errors = append(errors, SyntaxError{
			File:    file,
			Line:    1,
			Column:  1,
			Message: errStr,
		})
	}

	return errors
}
