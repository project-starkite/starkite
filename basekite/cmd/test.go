package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
	"github.com/spf13/cobra"
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

	// Sandbox engagement for `kite test`: each test file runs in its own
	// sandbox process. When the parent invocation has a sandbox engaged
	// AND the user gave a directory of multiple files, fork one child
	// kite per file, each with the sandbox engaged via env var. The
	// single-file path is handled by MaybeHandoffToSandbox below — that
	// produces one sandbox for that one file, which is the same outcome.
	profile, err := GetSandbox()
	if err != nil {
		return err
	}
	insideSandbox := os.Getenv(sandbox.InsideEnvVar) == "1"
	if !profile.IsZero() && !insideSandbox && len(testFiles) > 1 {
		return runTestFilesInSandbox(testFiles)
	}

	if handled, err := MaybeHandoffToSandbox(context.Background()); handled || err != nil {
		return err
	}

	perms, err := GetPermissions()
	if err != nil {
		return err
	}

	startTime := time.Now()
	var results []testResult

	if testParallel > 1 && len(testFiles) > 1 {
		// Run test files in parallel
		results = runTestFilesParallel(testFiles, testParallel, perms)
	} else {
		// Run test files sequentially
		for _, testFile := range testFiles {
			fileResults := runTestFile(testFile, perms)
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

func runTestFilesParallel(testFiles []string, workers int, perms *libkite.PermissionConfig) []testResult {
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
	for range workers {
		wg.Go(func() {
			for testFile := range work {
				fileResults := runTestFile(testFile, perms)
				resultsChan <- fileResults
			}
		})
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

func runTestFile(testFile string, perms *libkite.PermissionConfig) []testResult {
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
	varStore, err := loadVarStore()
	if err != nil {
		return []testResult{{
			Name:   testFile,
			File:   testFile,
			Passed: false,
			Error:  err.Error(),
		}}
	}

	// Create module config
	moduleConfig := &libkite.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		TestMode: true,
		VarStore: varStore,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:   testFile,
		OutputFormat: "text",
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		TestMode:     true,
		Permissions:  perms,
		Registry:     registry,
	}

	// Create runtime
	rt, err := libkite.New(cfg)
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
	testCfg := libkite.TestConfig{
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

// runTestFilesInSandbox forks one child `kite test <file>` per test file,
// each running inside its own sandbox. This guarantees that test files do
// not share sandbox state (filesystem mutations in /tmp, in-sandbox
// listening ports, gVisor-internal kernel state).
//
// The sandbox value (built-in name, file path, or named profile) is
// forwarded via STARKITE_SECURITY_SANDBOX env var; the parent's --sandbox
// flag (if any) is translated to the same env var on the child so the
// child's resolution path is identical to a shebang-style invocation.
func runTestFilesInSandbox(testFiles []string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving kite binary: %w", err)
	}

	value := sandboxEngagementValue()
	if value == "" {
		// Defensive: the caller already checked profile.IsZero() == false.
		return fmt.Errorf("internal: sandbox profile resolved but no engagement value")
	}

	startTime := time.Now()
	failed := false

	workers := testParallel
	if workers <= 0 {
		workers = 1
	}
	if workers > len(testFiles) {
		workers = len(testFiles)
	}

	if workers == 1 {
		for _, file := range testFiles {
			if !runOneTestFileInSandbox(self, file, value) {
				failed = true
			}
		}
	} else {
		sem := make(chan struct{}, workers)
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, file := range testFiles {
			wg.Add(1)
			sem <- struct{}{}
			go func(f string) {
				defer wg.Done()
				defer func() { <-sem }()
				if !runOneTestFileInSandbox(self, f, value) {
					mu.Lock()
					failed = true
					mu.Unlock()
				}
			}(file)
		}
		wg.Wait()
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total: %s across %d file(s)\n", elapsed, len(testFiles))
	fmt.Println(strings.Repeat("=", 60))

	if failed {
		return fmt.Errorf("tests failed")
	}
	return nil
}

func runOneTestFileInSandbox(kiteBin, file, sandboxValue string) bool {
	fmt.Printf("\n--- %s ---\n", file)
	args := childTestArgs(file)
	cmd := exec.Command(kiteBin, args...)
	cmd.Dir = filepath.Dir(file)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), sandbox.ProfileEnvVar+"="+sandboxValue)
	if sandboxDriver != "" {
		cmd.Env = append(cmd.Env, sandbox.DriverEnvVar+"="+sandboxDriver)
	}
	return cmd.Run() == nil
}

// childTestArgs constructs the argv for a child `kite test <file>`
// invocation, forwarding the parent's flags so per-file behavior matches
// what the user requested. The --sandbox flag is intentionally not
// forwarded — sandbox engagement passes through the env var instead.
// --parallel is also dropped: each child runs a single file.
func childTestArgs(file string) []string {
	args := []string{"test", file}
	if testVerbose {
		args = append(args, "--verbose")
	}
	if testFilter != "" {
		args = append(args, "--run", testFilter)
	}
	if permissionsMode != "" {
		args = append(args, "--permissions="+permissionsMode)
	}
	if debugMode {
		args = append(args, "--debug")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if timeout != 300 {
		args = append(args, "--timeout="+strconv.Itoa(timeout))
	}
	for _, v := range variables {
		args = append(args, "--var="+v)
	}
	for _, vf := range varFiles {
		args = append(args, "--var-file="+vf)
	}
	return args
}

// sandboxEngagementValue returns whatever the user supplied to engage the
// sandbox: the --sandbox-profile flag value, the env var if no flag, or "default"
// if --sandboxed was used. Mirrors GetSandbox()'s precedence so children see the
// exact value the parent resolved against.
func sandboxEngagementValue() string {
	if sandboxProfile != "" {
		return sandboxProfile
	}
	if v := os.Getenv(sandbox.ProfileEnvVar); v != "" {
		return v
	}
	if sandboxed || sandboxDriver != "" {
		return sandbox.DefaultProfileName
	}
	return ""
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
