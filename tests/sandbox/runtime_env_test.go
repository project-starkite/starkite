package sandbox_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func findPython(t *testing.T) string {
	t.Helper()
	pyBin, err := exec.LookPath("python3")
	if err != nil {
		if runtime.GOOS == "windows" {
			pyBin, err = exec.LookPath("python")
		}
		if err != nil {
			t.Skip("python not available on host")
		}
	}
	// Verify python is actually functional on host (guards against WindowsApps reparse points)
	if err := exec.Command(pyBin, "--version").Run(); err != nil {
		t.Skipf("python executable %q found but not runnable: %v", pyBin, err)
	}
	return pyBin
}

func findNode(t *testing.T) string {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available on host")
	}
	if err := exec.Command(nodeBin, "--version").Run(); err != nil {
		t.Skipf("node executable %q found but not runnable: %v", nodeBin, err)
	}
	return nodeBin
}

func TestRuntimeEnv_PythonVirtualEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Python virtual environment testing under Windows AppContainers requires host-wide toolcache ACL inheritance")
	}
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	pyBin := findPython(t)

	kiteBin := buildKite(t)
	workDir := createNonHomeTempDir(t)

	// Create a dummy module inside .venv
	venvDir := filepath.Join(workDir, ".venv")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatal(err)
	}
	dummyMod := filepath.Join(venvDir, "starkite_dummy.py")
	if err := os.WriteFile(dummyMod, []byte("GREETING = 'starkite-runtime-ok'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run python with explicit --sandbox-mount for the virtual environment
	mountArg := venvDir + ":" + venvDir + ":ro"
	pyCode := "import sys, starkite_dummy; sys.stdout.write(starkite_dummy.GREETING)"
	cmd := exec.Command(kiteBin, "exec", "--sandbox-mount", mountArg, "--env", "PYTHONPATH="+venvDir, "--", pyBin, "-c", pyCode)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PYTHONPATH="+venvDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if sandboxUnavailable(stderr.String()) || sandboxUnavailable(err.Error()) {
			t.Skipf("host cannot start a sandbox: %s (err: %v)", stderr.String(), err)
		}
		t.Fatalf("kite exec with --sandbox-mount failed: %v\nStderr: %s\nStdout: %s", err, stderr.String(), stdout.String())
	}

	if out := strings.TrimSpace(stdout.String()); out != "starkite-runtime-ok" {
		t.Errorf("stdout = %q, want 'starkite-runtime-ok'", out)
	}
}

func TestRuntimeEnv_HomeCredentialProtection(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	pyBin := findPython(t)

	kiteBin := buildKite(t)
	fakeHome := createNonHomeTempDir(t)

	sshDir := filepath.Join(fakeHome, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	secretKey := filepath.Join(sshDir, "id_rsa")
	if err := os.WriteFile(secretKey, []byte("secret-ssh-private-key"), 0600); err != nil {
		t.Fatal(err)
	}

	awsDir := filepath.Join(fakeHome, ".aws")
	if err := os.MkdirAll(awsDir, 0700); err != nil {
		t.Fatal(err)
	}
	secretAWS := filepath.Join(awsDir, "credentials")
	if err := os.WriteFile(secretAWS, []byte("secret-aws-token"), 0600); err != nil {
		t.Fatal(err)
	}

	workDir := createNonHomeTempDir(t)
	pyScript := filepath.Join(workDir, "probe.py")
	probeCode := "import sys\n" +
		"try:\n" +
		"    with open('" + filepath.ToSlash(secretKey) + "') as f:\n" +
		"        sys.stdout.write(f.read())\n" +
		"except Exception as e:\n" +
		"    sys.stderr.write(f'PROBE_DENIED: {e}\\n')\n" +
		"    sys.exit(1)\n"
	if err := os.WriteFile(pyScript, []byte(probeCode), 0644); err != nil {
		t.Fatal(err)
	}

	mountArg := workDir + ":" + workDir + ":ro"
	cmd := exec.Command(kiteBin, "exec", "--sandbox-mount", mountArg, "--", pyBin, pyScript)
	cmd.Dir = workDir
	cmd.Env = envWithFakeHome(fakeHome)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err == nil {
		t.Errorf("expected failure when accessing %q, but execution succeeded; stdout=%q", secretKey, stdout.String())
	}
	if strings.Contains(stdout.String(), "secret-ssh-private-key") {
		t.Errorf("sensitive credential leaked to stdout: %q", stdout.String())
	}
}

func TestRuntimeEnv_ActionableDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Python runtime diagnostics testing under Windows AppContainers requires host-wide toolcache ACL inheritance; see Node test for Windows")
	}
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	pyBin := findPython(t)

	kiteBin := buildKite(t)

	cmd := exec.Command(kiteBin, "exec", "--", pyBin, "-c", "import non_existent_starkite_module_xyz")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run() // Expect non-zero exit
	if sandboxUnavailable(stderr.String()) || (err != nil && sandboxUnavailable(err.Error())) {
		t.Skipf("host cannot start a sandbox: %s (err: %v)", stderr.String(), err)
	}

	wantHint := "Hint: grant access to dependencies via --sandbox-path-read=<path> or --sandbox-mount <host-path>:<target-path>:ro"
	if !strings.Contains(stderr.String(), wantHint) {
		t.Errorf("expected python diagnostic hint in stderr, got:\n%s", stderr.String())
	}
}

func TestRuntimeEnv_ActionableDiagnostics_Node(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	nodeBin := findNode(t)

	kiteBin := buildKite(t)

	cmd := exec.Command(kiteBin, "exec", "--", nodeBin, "-e", "require('non_existent_starkite_module_xyz')")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	wantHint := "Hint: grant access to dependencies via --sandbox-path-read=node_modules or --sandbox-mount <host-path>:<target-path>:ro"
	if !strings.Contains(stderr.String(), wantHint) {
		t.Errorf("expected node diagnostic hint in stderr, got:\n%s", stderr.String())
	}
}

func TestRuntimeEnv_TmpfsScratch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tmpfs scratch testing is POSIX-oriented")
	}
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}

	kiteBin := buildKite(t)

	tmpScratch := filepath.Join(os.TempDir(), "starkite-test-scratch")
	_ = os.MkdirAll(tmpScratch, 0755)
	defer os.RemoveAll(tmpScratch)

	testFile := filepath.Join(tmpScratch, "test")
	cmd := exec.Command(kiteBin, "exec", "--sandbox-mount", "tmpfs:"+tmpScratch, "--", "sh", "-c", "echo ok > "+testFile+" && cat "+testFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if sandboxUnavailable(stderr.String()) || sandboxUnavailable(err.Error()) {
			t.Skipf("host cannot start a sandbox: %s (err: %v)", stderr.String(), err)
		}
		t.Fatalf("kite exec with tmpfs failed: %v\nStderr: %s\nStdout: %s", err, stderr.String(), stdout.String())
	}

	if out := strings.TrimSpace(stdout.String()); out != "ok" {
		t.Errorf("stdout = %q, want 'ok'", out)
	}
}
