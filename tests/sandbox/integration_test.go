// Package sandbox_test drives the .star sandbox integration tests by
// invoking `kite test <file>` against tests/sandbox/*_test.star from a
// clean temp directory (so credential-isolation tests aren't fooled by
// $HOME being the $CWD bind).
//
// Engagement paths exercised:
//   - Default sandbox engagement (enabled by default)
//   - --sandbox-profile CLI flag (with = and space)
//   - --sandbox-net CLI flag
//   - STARKITE_SANDBOX_PROFILE env var
//   - --sandbox-driver CLI flag and STARKITE_SANDBOX_DRIVER env var
//
// Skipped unless STARKITE_SANDBOX_INTEGRATION=1.
package sandbox_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const perTestTimeout = 60 * time.Second

// engagement picks how the test driver tells kite to engage the sandbox.
type engagement int

const (
	viaDefaultProfile engagement = iota
	viaProfileFlag
	viaProfileFlagSpace
	viaEnv
	viaNetFlag
	viaDriverFlag
	viaDriverAndProfileFlag
	viaDriverEnv
)

func TestSandboxNetRung_flag(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "sandbox-net", viaProfileFlag)
}

func TestSandboxNetRung_flagSpace(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "sandbox-net", viaProfileFlagSpace)
}

func TestSandboxNetComposableFlag(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "sandbox-path", viaProfileFlag, "--sandbox-net")
}

func TestSandboxPathRung_flag(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "sandbox-path", viaProfileFlag)
}

func TestSandboxPathRung_flagSpace(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "sandbox-path", viaProfileFlagSpace)
}

func TestSandboxExecWithPathRead_flag(t *testing.T) {
	home, _ := os.UserHomeDir()
	runStarTest(t, "sandbox_host_test.star", "sandbox-exec", viaProfileFlag, "--sandbox-path-read="+home, "--sandbox-path-read=/usr")
}

func TestSandboxDefaultIsEnabled(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "default", viaDefaultProfile, "--sandbox-path-write=.")
}

func TestSandboxDriverAndProfile_flag(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "sandbox-path", viaDriverAndProfileFlag)
}

func TestSandboxDriverOnly_flag(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "default", viaDriverFlag, "--sandbox-path-write=.")
}

func TestSandboxDriver_env(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "sandbox-path", viaDriverEnv)
}

func TestSandboxModule_star(t *testing.T) {
	runStarTest(t, "sandbox_module_test.star", "sandbox-net", viaProfileFlag)
}

// Env-var engagement is the shebang-style path. Exercised on the sandbox-path rung.
func TestSandboxPathRung_env(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "sandbox-path", viaEnv)
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

	fakeHome := t.TempDir()
	cmd := exec.CommandContext(ctx, kite, "test", workDir, "--sandbox-profile=sandbox-path", "--env-inherit")
	cmd.Dir = workDir
	var filteredEnv []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "HOME=") && !strings.HasPrefix(e, "USERPROFILE=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	cmd.Env = append(filteredEnv, "HOME="+fakeHome, "USERPROFILE="+fakeHome)
	out, err := cmd.CombinedOutput()
	t.Logf("kite test --sandbox-profile=sandbox-path (multi-file) output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping. "+
				"This is a host-capability limitation, not a test failure. Output:\n%s", out)
		}
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
func runStarTest(t *testing.T, scriptName, profile string, eng engagement, extraArgs ...string) {
	t.Helper()

	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
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

	// Stage host facts the star tests cannot discover from inside the
	// sandbox (no $HOME env there).
	hostinfo := fmt.Sprintf("{\"home\": %q}", home)
	if err := os.WriteFile(filepath.Join(workDir, "hostinfo.json"), []byte(hostinfo), 0o644); err != nil {
		t.Fatalf("staging hostinfo.json: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	fakeHome := t.TempDir()
	args := []string{"test", dstPath, "--env-inherit"}
	args = append(args, extraArgs...)
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "HOME=") && !strings.HasPrefix(e, "USERPROFILE=") {
			env = append(env, e)
		}
	}
	env = append(env, "HOME="+fakeHome, "USERPROFILE="+fakeHome)
	var label string
	switch eng {
	case viaDefaultProfile:
		label = "default"
	case viaProfileFlag:
		args = append(args, "--sandbox-profile="+profile)
		label = "--sandbox-profile=" + profile
	case viaProfileFlagSpace:
		args = append(args, "--sandbox-profile", profile)
		label = "--sandbox-profile " + profile
	case viaNetFlag:
		args = append(args, "--sandbox-net")
		label = "--sandbox-net"
	case viaEnv:
		env = append(env, "STARKITE_SANDBOX_PROFILE="+profile)
		label = "STARKITE_SANDBOX_PROFILE=" + profile
	case viaDriverFlag:
		args = append(args, "--sandbox-driver=auto")
		label = "--sandbox-driver=auto"
	case viaDriverAndProfileFlag:
		args = append(args, "--sandbox-profile="+profile, "--sandbox-driver=auto")
		label = "--sandbox-profile=" + profile + " --sandbox-driver=auto"
	case viaDriverEnv:
		env = append(env, "STARKITE_SANDBOX_PROFILE="+profile, "STARKITE_SANDBOX_DRIVER=auto")
		label = "STARKITE_SANDBOX_PROFILE=" + profile + " STARKITE_SANDBOX_DRIVER=auto"
	default:
		t.Fatalf("unknown engagement %d", eng)
	}

	cmd := exec.CommandContext(ctx, kite, args...)
	cmd.Dir = workDir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	t.Logf("kite test (%s) output:\n%s", label, out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox (%s); skipping. "+
				"This is a host-capability limitation, not a test failure. Output:\n%s", label, out)
		}
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

var buildKiteOnce = sync.OnceValues(func() (string, error) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		return "", fmt.Errorf("resolving repo root: %w", err)
	}
	dir, err := os.MkdirTemp("", "kite-sandbox-bin-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}
	binName := "kite"
	if runtime.GOOS == "windows" {
		binName = "kite.exe"
	}
	out := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join(repoRoot, "kite")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building kite: %w\n%s", err, buildOut)
	}
	return out, nil
})

// buildKite builds the kite all-in-one binary once per test process and returns its path.
func buildKite(t *testing.T) string {
	t.Helper()
	out, err := buildKiteOnce()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// sandboxUnavailable reports whether kite's output indicates the host could not
// start a sandbox — a host-capability limitation (e.g. restricted
// cgroups/seccomp/user-namespaces on a CI runner, or unsupported in-process
// Landlock/Seatbelt prctl syscalls) that the static preflight cannot predict.
// Distinct from a genuine in-sandbox test failure, so the caller skips rather than fails.
func sandboxUnavailable(output string) bool {
	markers := []string{
		"cannot create sandbox",
		"cannot read client sync file",
		"waiting for sandbox to start",
		"failed to apply in-process sandbox",
		"prctl(PR_SET_NO_NEW_PRIVS) failed",
		"landlock_create_ruleset failed",
		"landlock_restrict_self failed",
		"landlock driver not supported",
	}
	for _, m := range markers {
		if strings.Contains(output, m) {
			return true
		}
	}
	return false
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
