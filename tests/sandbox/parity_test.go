package sandbox_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestParity_ZeroTrustDefaultBlocksAll asserts uniform zero-trust default behavior:
// ungranted network, filesystem write, and subprocess execution fail closed
// with actionable denial diagnostics across Linux, macOS, and Windows.
func TestParity_ZeroTrustDefaultBlocksAll(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := createNonHomeTempDir(t)

	denyScript := `
# 1. Outbound network request must fail closed
res_http = http.url("https://example.com").try_get()
assert(not res_http.ok, "http get should fail under default zero-trust")
assert("Hint: pass --sandbox-net" in res_http.error, "unexpected http error: %s" % res_http.error)

# 2. File write to unmounted path must fail closed
res_fs = fs.try_write_text("./unmounted_output.txt", "blocked-content")
assert(not res_fs.ok, "fs write should fail under default zero-trust")
assert("Hint: pass --sandbox-path-write" in res_fs.error, "unexpected fs error: %s" % res_fs.error)
`
	if err := os.WriteFile(filepath.Join(workDir, "deny_probe.star"), []byte(denyScript), 0o644); err != nil {
		t.Fatalf("writing deny_probe.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "run", "./deny_probe.star")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	t.Logf("kite run deny_probe.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run deny_probe.star failed: %v\nOutput: %s", err, out)
	}

	// 3. Subprocess execution must fail closed
	execDenyScript := `os.exec("echo parity-blocked")`
	if err := os.WriteFile(filepath.Join(workDir, "exec_deny.star"), []byte(execDenyScript), 0o644); err != nil {
		t.Fatalf("writing exec_deny.star: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, kite, "run", "./exec_deny.star")
	cmd2.Dir = workDir
	out2, err2 := cmd2.CombinedOutput()
	t.Logf("kite run exec_deny.star output:\n%s", out2)
	if err2 == nil {
		t.Fatal("expected kite run exec_deny.star to fail under default zero-trust profile")
	}
	output2 := string(out2)
	if !strings.Contains(output2, "permission denied: os.exec") && !strings.Contains(output2, "denied") {
		t.Errorf("expected exec permission error in output, got:\n%s", output2)
	}
	if !strings.Contains(output2, "Hint: pass --sandbox-exec-allow=") {
		t.Errorf("expected exec hint in output, got:\n%s", output2)
	}
}

// TestParity_ComposableFlagsEscalation asserts that composable CLI flags
// (--sandbox-path-read, --sandbox-path-write, --sandbox-exec-allow, and --sandbox-net)
// grant capabilities incrementally and cleanly across all operating systems.
func TestParity_ComposableFlagsEscalation(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := createNonHomeTempDir(t)

	if err := os.WriteFile(filepath.Join(workDir, "input.txt"), []byte("parity-input-data\n"), 0o644); err != nil {
		t.Fatalf("writing input.txt: %v", err)
	}

	escalationScript := `
# 1. Path read capability
data = fs.read_text("./input.txt")
assert(data.strip() == "parity-input-data", "unexpected input data: %s" % data)

# 2. Path write capability
fs.write_text("./output.txt", "parity-output-data")

# 3. Subprocess execution capability
def test_exec():
    if runtime.platform() == "windows":
        res_exec = os.exec("cmd.exe", ["/c", "echo parity-ok"])
        assert("parity-ok" in res_exec, "unexpected exec output: %s" % res_exec)
    else:
        res_exec = os.exec("uname")
        assert(len(res_exec.strip()) > 0, "unexpected exec output: %s" % res_exec)
test_exec()

# 4. Network capability: in-process HTTP server loopback
def handle_ping(req):
    return {"status": 200, "body": "pong-ok"}

srv = http.server()
srv.handle("/ping", handle_ping)
srv.start(port=0)
port = srv.port()
assert(port > 0, "http.server must bind valid port")

resp = http.url("http://127.0.0.1:%d/ping" % port).get()
assert(resp.status_code == 200, "expected 200, got %d" % resp.status_code)
assert(resp.get_text() == "pong-ok", "body mismatch: %s" % resp.get_text())
srv.shutdown()
`
	if err := os.WriteFile(filepath.Join(workDir, "escalation.star"), []byte(escalationScript), 0o644); err != nil {
		t.Fatalf("writing escalation.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	allowExecArg := "--sandbox-exec-allow=uname"
	if runtime.GOOS == "windows" {
		allowExecArg = "--sandbox-exec-allow=cmd.exe"
	}

	cmd := exec.CommandContext(ctx, kite, "run", "./escalation.star",
		"--sandbox-path-read=.",
		"--sandbox-path-write=.",
		allowExecArg,
		"--sandbox-net",
	)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	t.Logf("kite run escalation.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run escalation.star failed: %v\nOutput: %s", err, out)
	}

	outBytes, err := os.ReadFile(filepath.Join(workDir, "output.txt"))
	if err != nil {
		t.Fatalf("reading output.txt: %v", err)
	}
	if string(outBytes) != "parity-output-data" {
		t.Errorf("output.txt content = %q, want %q", string(outBytes), "parity-output-data")
	}
}

// TestParity_EnvScrubbingAndForwarding asserts that sensitive host environment variables
// are scrubbed by default, and selective forwarding (--env) functions identically across platforms.
func TestParity_EnvScrubbingAndForwarding(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := createNonHomeTempDir(t)

	envScript := `
# Scrubbed host variables must return empty string
aws = os.env("AWS_SECRET_ACCESS_KEY")
assert(aws == "", "AWS_SECRET_ACCESS_KEY must be scrubbed, got %s" % aws)

token = os.env("PARITY_SCRUBBED_TOKEN")
assert(token == "", "PARITY_SCRUBBED_TOKEN must be scrubbed, got %s" % token)

# Selectively forwarded host variable
fwd = os.env("FORWARDED_HOST_VAR")
assert(fwd == "host-secret-granted", "FORWARDED_HOST_VAR expected forwarded value, got %s" % fwd)

# Explicitly assigned variable
inj = os.env("INJECTED_CONFIG_VAR")
assert(inj == "custom-assigned-val", "INJECTED_CONFIG_VAR expected custom value, got %s" % inj)
`
	if err := os.WriteFile(filepath.Join(workDir, "env_parity.star"), []byte(envScript), 0o644); err != nil {
		t.Fatalf("writing env_parity.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "run", "./env_parity.star",
		"--sandbox-call-allow=os.env",
		"--env", "FORWARDED_HOST_VAR",
		"--env", "INJECTED_CONFIG_VAR=custom-assigned-val",
		"--debug",
	)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"AWS_SECRET_ACCESS_KEY=sensitive-aws-secret-key-12345",
		"PARITY_SCRUBBED_TOKEN=sensitive-api-token-67890",
		"FORWARDED_HOST_VAR=host-secret-granted",
	)

	out, err := cmd.CombinedOutput()
	t.Logf("kite run env_parity.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run env_parity.star failed: %v\nOutput: %s", err, out)
	}

	output := string(out)
	wantDiag := `[DEBUG] env "AWS_SECRET_ACCESS_KEY" requested by script is present on host but scrubbed by sandbox.`
	wantHint := `Hint: pass --env AWS_SECRET_ACCESS_KEY on CLI`
	if !strings.Contains(output, wantDiag) {
		t.Errorf("expected debug scrub diagnostic %q in output, got:\n%s", wantDiag, output)
	}
	if !strings.Contains(output, wantHint) {
		t.Errorf("expected scrub remediation hint %q in output, got:\n%s", wantHint, output)
	}
}

// TestParity_ScratchWXLockdown asserts Write-XOR-Execute enforcement: binaries or scripts
// placed in writable scratch directories (/tmp or %TEMP%) cannot be executed.
func TestParity_ScratchWXLockdown(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	workDir := createNonHomeTempDir(t)

	// Create an executable payload inside the OS temporary scratch directory
	tmpDir := "/tmp"
	payloadName := "wx_payload.sh"
	payloadContent := "#!/bin/sh\necho wx-executed\n"
	if runtime.GOOS == "windows" {
		tmpDir = os.TempDir()
		payloadName = "wx_payload.bat"
		payloadContent = "@echo wx-executed\r\n"
	}
	payloadPath := filepath.Join(tmpDir, payloadName)
	if err := os.WriteFile(payloadPath, []byte(payloadContent), 0o755); err != nil {
		t.Fatalf("creating scratch payload: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(payloadPath) })

	starScript := fmt.Sprintf(`
res = os.try_exec(%q)
assert(not res.ok or "permission denied" in res.stderr.lower() or "w^x" in res.stderr.lower() or "access is denied" in res.stderr.lower() or "not permitted" in res.stderr.lower(), "W^X violation: execution from writable scratch mount succeeded: " + str(res))
`, payloadPath)

	if err := os.WriteFile(filepath.Join(workDir, "wx_test.star"), []byte(starScript), 0o644); err != nil {
		t.Fatalf("writing wx_test.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "run", "./wx_test.star", "--sandbox-exec-allow=*")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	t.Logf("kite run wx_test.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run wx_test.star failed: %v\nOutput: %s", err, out)
	}
}

// TestParity_UserProfileIsolation asserts that executing a script located in $HOME
// or %USERPROFILE% confines access strictly to the script file without exposing the parent profile hierarchy.
func TestParity_UserProfileIsolation(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kite := buildKite(t)
	fakeHome := createNonHomeTempDir(t)

	secretPath := filepath.Join(fakeHome, "secret_profile.txt")
	if err := os.WriteFile(secretPath, []byte("sensitive-user-profile-data"), 0o600); err != nil {
		t.Fatalf("writing secret_profile.txt: %v", err)
	}

	homeScript := `
# Sibling file access inside $HOME / %USERPROFILE% must fail closed
res = fs.try_read_text("./secret_profile.txt")
assert(not res.ok, "sibling secret in $HOME must not be readable under user home isolation")
`
	scriptPath := filepath.Join(fakeHome, "home_probe.star")
	if err := os.WriteFile(scriptPath, []byte(homeScript), 0o644); err != nil {
		t.Fatalf("writing home_probe.star: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "run", "./home_probe.star")
	cmd.Dir = fakeHome

	cmd.Env = envWithFakeHome(fakeHome)

	out, err := cmd.CombinedOutput()
	t.Logf("kite run home_probe.star output:\n%s", out)
	if err != nil {
		if sandboxUnavailable(string(out)) {
			t.Skipf("host cannot start a sandbox; skipping: %s", out)
		}
		t.Fatalf("kite run home_probe.star failed: %v\nOutput: %s", err, out)
	}

	output := string(out)
	if strings.Contains(output, "sensitive-user-profile-data") {
		t.Fatalf("user profile isolation failed: secret data leaked in output: %s", output)
	}
}
