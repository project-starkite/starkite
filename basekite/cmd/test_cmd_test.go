package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRunTestsSuccess(t *testing.T) {
	dir := t.TempDir()
	content := `def test_simple_pass():
    assert(10 + 20 == 30, "arithmetic check")
`
	if err := os.WriteFile(filepath.Join(dir, "pass_test.star"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error on passing tests, got: %v", err)
	}
}

func TestRunTestsFailure(t *testing.T) {
	dir := t.TempDir()
	content := `def test_simple_fail():
    assert(1 == 2, "intentional failure")
`
	if err := os.WriteFile(filepath.Join(dir, "fail_test.star"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := runTests(nil, []string{dir})
	if err == nil {
		t.Fatal("expected error on failing tests, got nil")
	}
}

func TestRunTestsParallel(t *testing.T) {
	dir := t.TempDir()
	content1 := `def test_part1():
    assert(True, "part 1")
`
	content2 := `def test_part2():
    assert(True, "part 2")
`
	if err := os.WriteFile(filepath.Join(dir, "part1_test.star"), []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "part2_test.star"), []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	oldParallel := testParallel
	testParallel = 2
	defer func() { testParallel = oldParallel }()

	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error in parallel test runner, got: %v", err)
	}
}

func TestRunTestsFilter(t *testing.T) {
	dir := t.TempDir()
	content := `def test_alpha():
    assert(True, "alpha")
def test_beta():
    assert(False, "beta should not run")
`
	if err := os.WriteFile(filepath.Join(dir, "filter_test.star"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldFilter := testFilter
	testFilter = "alpha"
	defer func() { testFilter = oldFilter }()

	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error when filtering for passing test, got: %v", err)
	}
}

func TestRunTestsNoFiles(t *testing.T) {
	dir := t.TempDir()
	err := runTests(nil, []string{dir})
	if err != nil {
		t.Fatalf("expected nil error for empty directory, got: %v", err)
	}
}

func TestChildTestArgs(t *testing.T) {
	oldEnabled := sandboxEnabled
	oldProfile := sandboxProfile
	oldDriver := sandboxDriver
	oldPathRead := sandboxPathRead
	oldPathWrite := sandboxPathWrite
	oldCallAllow := sandboxCallAllow
	oldCallDeny := sandboxCallDeny
	oldExecAllow := sandboxExecAllow
	oldExecDeny := sandboxExecDeny
	oldNetAllow := sandboxNetAllow
	oldEnvFlags := envFlags
	oldEnvInherit := envInherit
	oldVerbose := testVerbose
	oldFilter := testFilter

	defer func() {
		sandboxEnabled = oldEnabled
		sandboxProfile = oldProfile
		sandboxDriver = oldDriver
		sandboxPathRead = oldPathRead
		sandboxPathWrite = oldPathWrite
		sandboxCallAllow = oldCallAllow
		sandboxCallDeny = oldCallDeny
		sandboxExecAllow = oldExecAllow
		sandboxExecDeny = oldExecDeny
		sandboxNetAllow = oldNetAllow
		envFlags = oldEnvFlags
		envInherit = oldEnvInherit
		testVerbose = oldVerbose
		testFilter = oldFilter
	}()

	sandboxEnabled = false
	sandboxProfile = "sandbox-path"
	sandboxDriver = "landlock"
	sandboxPathRead = []string{"/custom/read"}
	sandboxPathWrite = []string{"/custom/write"}
	sandboxCallAllow = []string{"os.env"}
	sandboxCallDeny = []string{"io.prompt"}
	sandboxExecAllow = []string{"echo"}
	sandboxExecDeny = []string{"rm"}
	sandboxNetAllow = []string{"*"}
	envFlags = []string{"FOO=bar"}
	envInherit = true
	testVerbose = true
	testFilter = "mytest"

	args := childTestArgs("foo_test.star")

	expectedSubstrings := []string{
		"test",
		"foo_test.star",
		"--verbose",
		"--run", "mytest",
		"--sandbox-enabled=false",
		"--sandbox-profile", "sandbox-path",
		"--sandbox-driver", "landlock",
		"--sandbox-path-read", "/custom/read",
		"--sandbox-path-write", "/custom/write",
		"--sandbox-call-allow", "os.env",
		"--sandbox-call-deny", "io.prompt",
		"--sandbox-exec-allow", "echo",
		"--sandbox-exec-deny", "rm",
		"--sandbox-net-allow=*",
		"--env=FOO=bar",
		"--env-inherit",
	}

	for _, sub := range expectedSubstrings {
		found := slices.Contains(args, sub)
		if !found {
			t.Errorf("childTestArgs missing %q, got: %v", sub, args)
		}
	}
}

func TestRunTestsParallel_WorkersClampAndBufferedOutput(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a_test.star")
	f2 := filepath.Join(dir, "b_test.star")
	if err := os.WriteFile(f1, []byte("def test_a():\n    print(\"ALPHA\")\n    assert(True)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("def test_b():\n    print(\"BETA\")\n    assert(True)\n"), 0644); err != nil {
		t.Fatal(err)
	}

	origVerbose := testVerbose
	testVerbose = true
	defer func() { testVerbose = origVerbose }()

	// Capture stdout from ordered flush in runTestFilesParallel with workers > len(files).
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	results := runTestFilesParallel(t.Context(), []string{f1, f2}, 8, nil, nil)
	_ = w.Close()
	os.Stdout = origStdout

	if len(results) != 2 || !results[0].Passed || !results[1].Passed {
		t.Fatalf("unexpected parallel results: %+v", results)
	}
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	_ = r.Close()
	out := string(buf[:n])
	alphaIdx := strings.Index(out, "ALPHA")
	betaIdx := strings.Index(out, "BETA")
	if alphaIdx < 0 || betaIdx < 0 || alphaIdx > betaIdx {
		t.Fatalf("expected deterministic per-file ordered output (ALPHA before BETA), got:\n%s", out)
	}
}
