package libkite

import "time"

// TestResult represents the result of a test execution.
type TestResult struct {
	Name     string
	Passed   bool
	Skipped  bool
	Duration time.Duration
	Error    error
}

// TestConfig holds test execution configuration.
type TestConfig struct {
	// Filter is a substring to match test names (empty = run all)
	Filter string
	// Verbose enables verbose output
	Verbose bool
}
