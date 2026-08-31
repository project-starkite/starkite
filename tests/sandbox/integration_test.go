// Package sandbox_test drives the .star sandbox integration tests by
// invoking `kite test <file>` against tests/sandbox/*_test.star from a
// clean temp directory (so credential-isolation tests aren't fooled by
// $HOME being the $CWD bind).
//
// Engagement paths exercised:
//   - --sandboxed CLI flag (boolean switch for default profile)
//   - --sandbox-profile CLI flag (with = and space)
//   - Shortcut aliases (--sandbox-opaque, --sandbox-net, --sandbox-net-access, --sandbox-host)
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
	"strings"
	"testing"
	"time"
)

const perTestTimeout = 60 * time.Second

// engagement picks how the test driver tells kite to engage the sandbox.
type engagement int

const (
	viaSandboxedFlag engagement = iota
	viaProfileFlag
	viaProfileFlagSpace
	viaEnv
	viaAliasOpaque
	viaAliasNet
	viaAliasNetAccess
	viaAliasHost
	viaDriverFlag
	viaDriverAndProfileFlag
	viaDriverEnv
)

func TestSandboxNetAccessRung_flag(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "net-access", viaProfileFlag)
}

func TestSandboxNetAccessRung_flagSpace(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "net-access", viaProfileFlagSpace)
}

func TestSandboxNetAccessRung_aliasNet(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "net-access", viaAliasNet)
}

func TestSandboxNetAccessRung_aliasNetAccess(t *testing.T) {
	runStarTest(t, "sandbox_netaccess_test.star", "net-access", viaAliasNetAccess)
}

func TestSandboxOpaqueRung_flag(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "opaque", viaProfileFlag)
}

func TestSandboxOpaqueRung_flagSpace(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "opaque", viaProfileFlagSpace)
}

func TestSandboxOpaqueRung_alias(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "opaque", viaAliasOpaque)
}

func TestSandboxHostRung_flag(t *testing.T) {
	runStarTest(t, "sandbox_host_test.star", "host", viaProfileFlag)
}

func TestSandboxHostRung_alias(t *testing.T) {
	runStarTest(t, "sandbox_host_test.star", "host", viaAliasHost)
}

func TestSandboxSandboxedFlagDefaultsToOpaque(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "default", viaSandboxedFlag)
}

func TestSandboxDriverAndProfile_flag(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "opaque", viaDriverAndProfileFlag)
}

func TestSandboxDriverOnly_flag(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "default", viaDriverFlag)
}

func TestSandboxDriver_env(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "opaque", viaDriverEnv)
}

func TestSandboxModule_star(t *testing.T) {
	runStarTest(t, "sandbox_module_test.star", "host", viaProfileFlag)
}

// Env-var engagement is the shebang-style path. Exercised on the opaque rung.
func TestSandboxOpaqueRung_env(t *testing.T) {
	runStarTest(t, "sandbox_opaque_test.star", "opaque", viaEnv)
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

	// allow-all keeps the permission layer out of the way so what the test
	// observes is the sandbox's isolation, not a permission denial.
	cmd := exec.CommandContext(ctx, kite, "test", workDir, "--sandbox-profile=opaque", "--permissions=allow-all")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	t.Logf("kite test --sandbox-profile=opaque (multi-file) output:\n%s", out)
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
func runStarTest(t *testing.T, scriptName, profile string, eng engagement) {
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

	// allow-all keeps the permission layer out of the way so what the test
	// observes is the sandbox's isolation, not a permission denial.
	args := []string{"test", dstPath, "--permissions=allow-all"}
	env := os.Environ()
	var label string
	switch eng {
	case viaSandboxedFlag:
		args = append(args, "--sandboxed")
		label = "--sandboxed"
	case viaProfileFlag:
		args = append(args, "--sandbox-profile="+profile)
		label = "--sandbox-profile=" + profile
	case viaProfileFlagSpace:
		args = append(args, "--sandbox-profile", profile)
		label = "--sandbox-profile " + profile
	case viaAliasOpaque:
		args = append(args, "--sandbox-opaque")
		label = "--sandbox-opaque"
	case viaAliasNet:
		args = append(args, "--sandbox-net")
		label = "--sandbox-net"
	case viaAliasNetAccess:
		args = append(args, "--sandbox-net-access")
		label = "--sandbox-net-access"
	case viaAliasHost:
		args = append(args, "--sandbox-host")
		label = "--sandbox-host"
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

// buildKite builds the kite all-in-one binary into a
// t.TempDir() and returns its path.
func buildKite(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	out := filepath.Join(t.TempDir(), "kite")

	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join(repoRoot, "kite")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building kite: %v\n%s", err, buildOut)
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
