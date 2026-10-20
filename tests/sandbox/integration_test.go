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

func tempDirOutsideHome(t *testing.T) string {
	t.Helper()
	// On Linux, t.TempDir() defaults to /tmp, which is the sandbox's designated scratch mount.
	// To verify that CWD isolation works under deny-all (and isn't masked by the /tmp scratch rule),
	// stage workDir in /var/tmp on Linux.
	if runtime.GOOS == "linux" {
		fallback, err := os.MkdirTemp("/var/tmp", "starkite-zt-*")
		if err == nil {
			t.Cleanup(func() { os.RemoveAll(fallback) })
			return fallback
		}
	}
	if runtime.GOOS == "windows" {
		tmpBase := os.Getenv("SystemRoot") + `\Temp`
		fallback, err := os.MkdirTemp(tmpBase, "starkite-zt-*")
		if err == nil {
			t.Cleanup(func() { os.RemoveAll(fallback) })
			return fallback
		}
	}
	dir := t.TempDir()
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(dir, home) {
		fallback, err := os.MkdirTemp("", "starkite-zt-*")
		if err != nil {
			t.Fatalf("creating temp dir outside HOME: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(fallback) })
		return fallback
	}
	return dir
}

// TestZeroTrustDefault_EndToEnd verifies that `kite run` defaults to zero-trust
// confinement ('deny-all'): static AST load() closures are pre-loaded into memory
// and work without CWD access, while filesystem reads, subprocess execution,
// network egress, and ambient host environment variables are strictly blocked with
// actionable denial hints.
func TestZeroTrustDefault_EndToEnd(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := tempDirOutsideHome(t)

	helperContent := `MSG = "loaded-from-closure"`
	if err := os.WriteFile(filepath.Join(workDir, "helper.star"), []byte(helperContent), 0o644); err != nil {
		t.Fatalf("writing helper.star: %v", err)
	}

	if err := os.WriteFile(filepath.Join(workDir, "secret.txt"), []byte("sensitive-data\n"), 0o644); err != nil {
		t.Fatalf("writing secret.txt: %v", err)
	}

	mainContent := `load("./helper.star", "helper")

assert(helper.MSG == "loaded-from-closure", "expected MSG from closure")

# 1. fs access to secret.txt should fail with actionable Hint
res_fs = fs.try_read_text("./secret.txt")
assert(not res_fs.ok, "fs read should fail under default deny-all")
assert("Hint: pass --sandbox-path-read=." in res_fs.error, "unexpected fs error: %s" % res_fs.error)

# 2. http access should fail with actionable Hint
res_http = http.url("https://example.com").try_get()
assert(not res_http.ok, "http get should fail under default deny-all")
assert("Hint: pass --sandbox-net" in res_http.error, "unexpected http error: %s" % res_http.error)

# 3. os.env should fail with actionable Hint under default deny-all
res_env = os.try_env("AMBIENT_HOST_SECRET")
assert(not res_env.ok, "os.env should fail under default deny-all")
assert("Hint: pass --sandbox-call-allow=os.env" in res_env.error, "unexpected env error: %s" % res_env.error)
`
	if err := os.WriteFile(filepath.Join(workDir, "main.star"), []byte(mainContent), 0o644); err != nil {
		t.Fatalf("writing main.star: %v", err)
	}

	execFailContent := `os.exec("echo hello")`
	if err := os.WriteFile(filepath.Join(workDir, "exec_fail.star"), []byte(execFailContent), 0o644); err != nil {
		t.Fatalf("writing exec_fail.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	// 1. main.star: pure compute, closure load, try_ assertions
	cmd := exec.CommandContext(ctx, kite, "run", "./main.star")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "AMBIENT_HOST_SECRET=leaked")
	if runtime.GOOS == "darwin" && os.Getenv("STARKITE_SANDBOX_DRIVER") == "" {
		cmd.Env = append(cmd.Env, "STARKITE_SANDBOX_DRIVER=seatbelt")
	}

	out, err := cmd.CombinedOutput()
	t.Logf("kite run main.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run main.star failed: %v\nOutput: %s", err, out)
	}

	// 2. exec_fail.star: os.exec without flags must fail with actionable Hint
	ctx2, cancel2 := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, kite, "run", "./exec_fail.star")
	cmd2.Dir = workDir
	if runtime.GOOS == "darwin" && os.Getenv("STARKITE_SANDBOX_DRIVER") == "" {
		cmd2.Env = append(os.Environ(), "STARKITE_SANDBOX_DRIVER=seatbelt")
	}

	out2, err2 := cmd2.CombinedOutput()
	t.Logf("kite run exec_fail.star output:\n%s", out2)
	if err2 == nil {
		t.Fatal("expected kite run exec_fail.star to fail under default deny-all profile")
	}
	output2 := string(out2)
	wantExecErr := "permission denied: os.exec"
	wantExecHint := "Hint: pass --sandbox-exec-allow=echo"
	if !strings.Contains(output2, wantExecErr) {
		t.Errorf("expected exec error %q in output, got:\n%s", wantExecErr, output2)
	}
	if !strings.Contains(output2, wantExecHint) {
		t.Errorf("expected exec hint %q in output, got:\n%s", wantExecHint, output2)
	}
}

// TestZeroTrustComposableFlags_EndToEnd verifies that composable flags
// (--sandbox-path-write, --sandbox-exec-allow, --env) correctly open up capabilities
// on top of the zero-trust default.
func TestZeroTrustComposableFlags_EndToEnd(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := tempDirOutsideHome(t)

	if err := os.WriteFile(filepath.Join(workDir, "input.txt"), []byte("hello-input\n"), 0o644); err != nil {
		t.Fatalf("writing input.txt: %v", err)
	}

	workContent := `in_txt = fs.read_text("./input.txt")
assert(in_txt.strip() == "hello-input", "unexpected input: %s" % in_txt)

fs.write_text("./output.txt", "output-ok")

res = os.exec("echo composable-ok")
assert("composable-ok" in res, "unexpected exec stdout: %s" % res)

fwd = os.env("FORWARDED_VAR")
assert(fwd == "from-host", "unexpected FORWARDED_VAR: %s" % fwd)

inj = os.env("INJECTED_VAR")
assert(inj == "custom42", "unexpected INJECTED_VAR: %s" % inj)

ambient = os.env("UNFORWARDED_HOST_VAR")
assert(ambient == "", "expected ambient host var to be scrubbed, got: %s" % ambient)
`
	if err := os.WriteFile(filepath.Join(workDir, "work.star"), []byte(workContent), 0o644); err != nil {
		t.Fatalf("writing work.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "run", "./work.star",
		"--sandbox-path-write=.",
		"--sandbox-exec-allow=echo",
		"--env", "FORWARDED_VAR",
		"--env", "INJECTED_VAR=custom42",
	)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "FORWARDED_VAR=from-host", "UNFORWARDED_HOST_VAR=leaked")
	if runtime.GOOS == "darwin" && os.Getenv("STARKITE_SANDBOX_DRIVER") == "" {
		cmd.Env = append(cmd.Env, "STARKITE_SANDBOX_DRIVER=seatbelt")
	}

	out, err := cmd.CombinedOutput()
	t.Logf("kite run work.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run work.star failed: %v\nOutput: %s", err, out)
	}

	outBytes, err := os.ReadFile(filepath.Join(workDir, "output.txt"))
	if err != nil {
		t.Fatalf("output.txt not written: %v", err)
	}
	if string(outBytes) != "output-ok" {
		t.Errorf("output.txt content = %q, want %q", string(outBytes), "output-ok")
	}
}

// TestZeroTrustEnvScrubDiagnostic_EndToEnd verifies that when a script requests a host
// environment variable that was scrubbed by the sandbox, a diagnostic warning with
// actionable remediation is logged under --debug.
func TestZeroTrustEnvScrubDiagnostic_EndToEnd(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := tempDirOutsideHome(t)

	probeContent := `val = os.env("SCRUBBED_TOKEN")`
	if err := os.WriteFile(filepath.Join(workDir, "env_probe.star"), []byte(probeContent), 0o644); err != nil {
		t.Fatalf("writing env_probe.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "run", "./env_probe.star", "--sandbox-call-allow=os.env", "--debug")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "SCRUBBED_TOKEN=secret123")
	if runtime.GOOS == "darwin" && os.Getenv("STARKITE_SANDBOX_DRIVER") == "" {
		cmd.Env = append(cmd.Env, "STARKITE_SANDBOX_DRIVER=seatbelt")
	}

	out, err := cmd.CombinedOutput()
	t.Logf("kite run env_probe.star --debug output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run env_probe.star failed: %v\nOutput: %s", err, out)
	}

	output := string(out)
	wantDiag := `[DEBUG] env "SCRUBBED_TOKEN" requested by script is present on host but scrubbed by sandbox.`
	wantHint := `Hint: pass --env SCRUBBED_TOKEN on CLI`
	if !strings.Contains(output, wantDiag) {
		t.Errorf("expected debug diagnostic %q in output, got:\n%s", wantDiag, output)
	}
	if !strings.Contains(output, wantHint) {
		t.Errorf("expected hint %q in output, got:\n%s", wantHint, output)
	}
}

// TestZeroTrustModuleTrustBoundary_EndToEnd verifies that third-party module capability
// declarations in mod.yaml cannot self-escalate past the active sandbox profile,
// producing an actionable denial hint, and succeeds once granted by the operator.
func TestZeroTrustModuleTrustBoundary_EndToEnd(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := tempDirOutsideHome(t)

	modDir := filepath.Join(workDir, "netmod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatalf("creating netmod dir: %v", err)
	}

	modYAML := `namespace: acme
name: netmod
version: 0.1.0
sandbox:
  network: host
`
	if err := os.WriteFile(filepath.Join(modDir, "mod.yaml"), []byte(modYAML), 0o644); err != nil {
		t.Fatalf("writing mod.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "main.star"), []byte("ok = True\n"), 0o644); err != nil {
		t.Fatalf("writing main.star: %v", err)
	}

	appContent := `load("./netmod", "netmod")`
	if err := os.WriteFile(filepath.Join(workDir, "app.star"), []byte(appContent), 0o644); err != nil {
		t.Fatalf("writing app.star: %v", err)
	}

	// 1. Run under default zero-trust flags: must fail with capability boundary error
	ctx1, cancel1 := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel1()

	cmd1 := exec.CommandContext(ctx1, kite, "run", "./app.star")
	cmd1.Dir = workDir
	if runtime.GOOS == "darwin" && os.Getenv("STARKITE_SANDBOX_DRIVER") == "" {
		cmd1.Env = append(os.Environ(), "STARKITE_SANDBOX_DRIVER=seatbelt")
	}

	out1, err1 := cmd1.CombinedOutput()
	t.Logf("kite run app.star (default) output:\n%s", out1)
	if err1 == nil {
		t.Fatal("expected kite run app.star to fail under default deny-all profile")
	}

	output1 := string(out1)
	wantModErr := `module "acme/netmod" requires capabilities exceeding active profile "default"`
	wantModHint := `Hint: rerun with required capabilities: kite run --sandbox-net`
	if !strings.Contains(output1, wantModErr) {
		t.Errorf("expected module error %q in output, got:\n%s", wantModErr, output1)
	}
	if !strings.Contains(output1, wantModHint) {
		t.Errorf("expected module hint %q in output, got:\n%s", wantModHint, output1)
	}

	// 2. Rerun with --sandbox-net granted by operator: must succeed
	ctx2, cancel2 := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, kite, "run", "./app.star", "--sandbox-net")
	cmd2.Dir = workDir
	if runtime.GOOS == "darwin" && os.Getenv("STARKITE_SANDBOX_DRIVER") == "" {
		cmd2.Env = append(os.Environ(), "STARKITE_SANDBOX_DRIVER=seatbelt")
	}

	out2, err2 := cmd2.CombinedOutput()
	t.Logf("kite run app.star (--sandbox-net) output:\n%s", out2)
	if err2 != nil {
		if sandboxUnavailable(string(out2)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out2)
		}
		t.Fatalf("kite run app.star --sandbox-net failed: %v\nOutput: %s", err2, out2)
	}
}
