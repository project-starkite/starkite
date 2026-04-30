package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/basekite/varstore"
	"github.com/project-starkite/starkite/starbase"
)

var (
	testVerbose  bool
	testParallel int
	testFilter   string
)

var testCmd = &cobra.Command{
	Use:   "test <path>",
	Short: "Run starkite tests",
	Long: `Run test scripts in the specified directory.

Test files should end with _test.star and contain functions prefixed with test_.
Tests can optionally define setup() and teardown() functions that run before
and after each test.

Test functions:
  test_*()    - Test functions (required)
  setup()     - Runs before each test (optional)
  teardown()  - Runs after each test (optional)

Built-in test functions:
  assert(condition, message)  - Fail if condition is false
  skip()                      - Skip the current test
  skip("reason")              - Skip with a reason

Examples:
  # Run all tests in a directory
  kite test ./tests/

  # Run a single test file
  kite test ./tests/math_test.star

  # Run tests with verbose output
  kite test ./tests/ --verbose

  # Run tests matching a pattern
  kite test ./tests/ --run string

  # Run test files in parallel
  kite test ./tests/ --parallel 4
`,
	Args: cobra.ExactArgs(1),
	RunE: runTests,
}

func init() {
	testCmd.Flags().BoolVarP(&testVerbose, "verbose", "v", false, "Verbose test output")
	testCmd.Flags().IntVarP(&testParallel, "parallel", "p", 1, "Number of parallel test file runners")
	testCmd.Flags().StringVar(&testFilter, "run", "", "Run only tests matching this substring")
	rootCmd.AddCommand(testCmd)
}

type testResult struct {
	Name     string
	File     string
	Passed   bool
	Skipped  bool
	Duration time.Duration
	Error    string
}

func runTests(cmd *cobra.Command, args []string) error {
	testPath := args[0]

	// Find all test files
	testFiles, err := findTestFiles(testPath)
	if err != nil {
		return fmt.Errorf("failed to find test files: %w", err)
	}

	if len(testFiles) == 0 {
		fmt.Println("No test files found")
		return nil
	}

	fmt.Printf("Found %d test file(s)\n", len(testFiles))
	if testFilter != "" {
		fmt.Printf("Filter: %s\n", testFilter)
	}

	startTime := time.Now()
	var results []testResult

	if testParallel > 1 && len(testFiles) > 1 {
		// Run test files in parallel
		results = runTestFilesParallel(testFiles, testParallel)
	} else {
		// Run test files sequentially
		for _, testFile := range testFiles {
			fileResults := runTestFile(testFile)
			results = append(results, fileResults...)
		}
	}

	// Print summary
	elapsed := time.Since(startTime)
	printTestSummary(results, elapsed)

	// Return error if any tests failed
	for _, r := range results {
		if !r.Passed && !r.Skipped {
			return fmt.Errorf("tests failed")
		}
	}

	return nil
}

func runTestFilesParallel(testFiles []string, workers int) []testResult {
	// Create work channel
	work := make(chan string, len(testFiles))
	for _, f := range testFiles {
		work <- f
	}
	close(work)

	// Create results channel
	resultsChan := make(chan []testResult, len(testFiles))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for testFile := range work {
				fileResults := runTestFile(testFile)
				resultsChan <- fileResults
			}
		}()
	}

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var allResults []testResult
	for fileResults := range resultsChan {
		allResults = append(allResults, fileResults...)
	}

	return allResults
}

func findTestFiles(path string) ([]string, error) {
	var files []string

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		// Single file - allow any .star file for direct testing
		if strings.HasSuffix(path, ".star") {
			return []string{path}, nil
		}
		return nil, fmt.Errorf("not a .star file: %s", path)
	}

	// Walk directory
	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(p, "_test.star") {
			files = append(files, p)
		}
		return nil
	})

	return files, err
}

func runTestFile(testFile string) []testResult {
	if testVerbose {
		fmt.Printf("Running %s\n", testFile)
	}

	// Read file
	content, err := os.ReadFile(testFile)
	if err != nil {
		return []testResult{{
			Name:   testFile,
			File:   testFile,
			Passed: false,
			Error:  fmt.Sprintf("failed to read file: %v", err),
		}}
	}

	// Create and populate variable store
	varStore := varstore.New()
	varStore.LoadFromEnv()
	_ = varStore.LoadFromCLI(variables)

	// Create module config
	moduleConfig := &starbase.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		TestMode: true,
		VarStore: varStore,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	// Create runtime configuration
	cfg := &starbase.Config{
		ScriptPath:   testFile,
		OutputFormat: "text",
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		TestMode:     true,
		Permissions:  GetPermissions(),
		Registry:     registry,
	}

	// Create runtime
	rt, err := starbase.New(cfg)
	if err != nil {
		return []testResult{{
			Name:   testFile,
			File:   testFile,
			Passed: false,
			Error:  fmt.Sprintf("failed to create runtime: %v", err),
		}}
	}
	defer rt.Cleanup()

	ctx, cancel := execContext(timeout)
	defer cancel()

	// Execute and collect test results with filter
	testCfg := starbase.TestConfig{
		Filter:  testFilter,
		Verbose: testVerbose,
	}
	results, err := rt.ExecuteTestsWithConfig(ctx, string(content), testCfg)
	if err != nil {
		return []testResult{{
			Name:   testFile,
			File:   testFile,
			Passed: false,
			Error:  fmt.Sprintf("failed to execute tests: %v", err),
		}}
	}

	// Convert to testResult
	var testResults []testResult
	for _, r := range results {
		tr := testResult{
			Name:     r.Name,
			File:     testFile,
			Passed:   r.Passed,
			Skipped:  r.Skipped,
			Duration: r.Duration,
		}
		if r.Error != nil {
			tr.Error = r.Error.Error()
		}
		testResults = append(testResults, tr)

		// Print verbose output
		if testVerbose {
			if tr.Skipped {
				reason := ""
				if tr.Error != "" {
					reason = " (" + tr.Error + ")"
				}
				fmt.Printf("  - %s [SKIPPED]%s\n", tr.Name, reason)
			} else if tr.Passed {
				fmt.Printf("  ✓ %s (%s)\n", tr.Name, tr.Duration)
			} else {
				fmt.Printf("  ✗ %s: %s\n", tr.Name, tr.Error)
			}
		}
	}

	return testResults
}

func printTestSummary(results []testResult, elapsed time.Duration) {
	passed := 0
	failed := 0
	skipped := 0

	for _, r := range results {
		if r.Skipped {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Println(strings.Repeat("=", 60))
	if skipped > 0 {
		fmt.Printf("Tests: %d passed, %d failed, %d skipped, %d total\n", passed, failed, skipped, len(results))
	} else {
		fmt.Printf("Tests: %d passed, %d failed, %d total\n", passed, failed, len(results))
	}
	fmt.Printf("Time:  %s\n", elapsed.Round(time.Millisecond))
	fmt.Println(strings.Repeat("=", 60))

	// Print failed tests
	if failed > 0 {
		fmt.Println("\nFailed tests:")
		for _, r := range results {
			if !r.Passed && !r.Skipped {
				fmt.Printf("  ✗ %s (%s)\n", r.Name, r.File)
				if r.Error != "" {
					fmt.Printf("    Error: %s\n", r.Error)
				}
			}
		}
	}
}
