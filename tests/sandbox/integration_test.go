//go:build linux

// Package sandbox_test drives the .star sandbox integration tests by
// invoking `kite test <file>` against tests/sandbox/*_test.star from a
// clean temp directory (so credential-isolation tests aren't fooled by
// $HOME being the $CWD bind).
//
// Two engagement paths are exercised:
//   - --sandbox CLI flag (the explicit kite-invocation path)
//   - STARKITE_SECURITY_SANDBOX env var (the shebang-style path)
//
// Skipped on non-Linux (build tag).
// Skipped unless STARKITE_SANDBOX_INTEGRATION=1 — these tests build
// allkite, spawn kite under gVisor, and depend on the kernel allowing
// unprivileged user namespaces. CI opts in explicitly; local `go test
// ./...` doesn't trigger them.
package sandbox_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const perTestTimeout = 60 * time.Second

// engagement picks how the test driver tells kite to engage the sandbox.
// Both paths are documented user-facing: the flag for explicit CLI use,
// the env var for shebang-launched scripts.
type engagement int

const (
	viaFlag engagement = iota
	viaEnv
)

func TestSandboxDefaultProfile_flag(t *testing.T) {
	runStarTest(t, "sandbox_default_test.star", "default", viaFlag)
}

func TestSandboxStrictProfile_flag(t *testing.T) {
	runStarTest(t, "sandbox_strict_test.star", "strict", viaFlag)
}

// Env-var engagement is the shebang-style path. Exercised on the strict
// profile; default-via-env would just duplicate the flag-path coverage.
func TestSandboxStrictProfile_env(t *testing.T) {
	runStarTest(t, "sandbox_strict_test.star", "strict", viaEnv)
}

// TestSandboxPerTestFile verifies that `kite test <dir>` with a sandbox
// engaged runs each test file in its own sandbox process. The "multi"
// directory holds two test files; we expect to see two "--- <file> ---"
// banner lines in the parent output (one per child) and a "Total: ...
// across 2 file(s)" footer.
func TestSandboxPerTestFile(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	if reason := unprivilegedUsernsBlocked(); reason != "" {
		t.Skipf("kernel restricts unprivileged user namespaces: %s", reason)
	}

	kite := buildKite(t)

	workDir := t.TempDir()
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(workDir, home) {
		t.Fatalf("temp dir %s is under $HOME — sandbox isolation tests would be confounded", workDir)
	}
	for _, name := range []string{"multi_a_test.star", "multi_b_test.star"} {
		src := mustAbs(t, filepath.Join("multi", name))
		dst := filepath.Join(workDir, name)
		if err := copyFile(src, dst); err != nil {
			t.Fatalf("staging %s: %v", name, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "test", workDir, "--sandbox=strict")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	t.Logf("kite test --sandbox=strict (multi-file) output:\n%s", out)
	if err != nil {
		t.Fatalf("kite test multi failed: %v", err)
	}

	output := string(out)
	for _, want := range []string{
		"--- " + filepath.Join(workDir, "multi_a_test.star") + " ---",
		"--- " + filepath.Join(workDir, "multi_b_test.star") + " ---",
		"Total: ",
		"across 2 file(s)",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output", want)
		}
	}
	if strings.Contains(output, "failed") && !strings.Contains(output, "0 failed") {
		t.Errorf("kite test reports failures; see output above")
	}
}

// runStarTest builds kite, copies the named .star file into a non-$HOME
// temp dir, and runs `kite test <file>` with the sandbox engaged via
// either the --sandbox flag or STARKITE_SECURITY_SANDBOX env var.
// Asserts the printed test summary shows passes and zero failures.
func runStarTest(t *testing.T, scriptName, profile string, eng engagement) {
	t.Helper()

	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	if reason := unprivilegedUsernsBlocked(); reason != "" {
		t.Skipf("kernel restricts unprivileged user namespaces: %s\n"+
			"To run these tests: install kite's AppArmor profile, or\n"+
			"  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0",
			reason)
	}

	kite := buildKite(t)

	// Run from a temp dir explicitly OUTSIDE $HOME so the $CWD bind
	// doesn't accidentally expose ~/.aws, ~/.ssh, etc. to the script.
	workDir := t.TempDir()
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(workDir, home) {
		t.Fatalf("temp dir %s is under $HOME — sandbox isolation tests would be confounded", workDir)
	}

	// The sandbox profiles only mount $CWD. The test script must live
	// inside $CWD so kite inside the sandbox can find it.
	srcPath := mustAbs(t, scriptName)
	dstPath := filepath.Join(workDir, scriptName)
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("staging test script into workdir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	args := []string{"test", dstPath}
	env := os.Environ()
	var label string
	switch eng {
	case viaFlag:
		args = append(args, "--sandbox="+profile)
		label = "--sandbox=" + profile
	case viaEnv:
		env = append(env, "STARKITE_SECURITY_SANDBOX="+profile)
		label = "STARKITE_SECURITY_SANDBOX=" + profile
	default:
		t.Fatalf("unknown engagement %d", eng)
	}

	cmd := exec.CommandContext(ctx, kite, args...)
	cmd.Dir = workDir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	t.Logf("kite test (%s) output:\n%s", label, out)
	if err != nil {
		t.Fatalf("kite test (%s) failed: %v", label, err)
	}

	output := string(out)
	if !strings.Contains(output, "passed") {
		t.Errorf("expected 'passed' summary in output")
	}
	if strings.Contains(output, "failed") && !strings.Contains(output, "0 failed") {
		t.Errorf("kite test reports failures; see output above")
	}
}

// buildKite builds the allkite binary (the kite all-in-one) into a
// t.TempDir() and returns its path.
func buildKite(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	out := filepath.Join(t.TempDir(), "kite")

	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join(repoRoot, "allkite")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building kite: %v\n%s", err, buildOut)
	}
	return out
}

// unprivilegedUsernsBlocked returns a non-empty reason string when the
// kernel state prevents unprivileged user namespaces (the precondition
// gVisor's rootless mode needs). Empty string means preflight is happy.
func unprivilegedUsernsBlocked() string {
	if v, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns"); err == nil {
		if strings.TrimSpace(string(v)) == "1" {
			return "apparmor_restrict_unprivileged_userns=1"
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		// Unreadable but present — odd; let gVisor surface the real error.
	}
	if v, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		if strings.TrimSpace(string(v)) == "0" {
			return "unprivileged_userns_clone=0"
		}
	}
	return ""
}

func mustAbs(t *testing.T, rel string) string {
	t.Helper()
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", rel, err)
	}
	return abs
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
