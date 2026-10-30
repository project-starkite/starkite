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

	// Run python under python-env profile with PYTHONPATH pointing to .venv
	pyCode := "import sys, starkite_dummy; sys.stdout.write(starkite_dummy.GREETING)"
	cmd := exec.Command(kiteBin, "exec", "--sandbox-profile=python-env", "--env", "PYTHONPATH="+venvDir, "--", pyBin, "-c", pyCode)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "PYTHONPATH="+venvDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if sandboxUnavailable(stderr.String()) || sandboxUnavailable(err.Error()) {
			t.Skipf("host cannot start a sandbox: %s (err: %v)", stderr.String(), err)
		}
		t.Fatalf("kite exec python-env failed: %v\nStderr: %s\nStdout: %s", err, stderr.String(), stdout.String())
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

	// Python script attempting to read shielded credential paths
	pyScript := filepath.Join(fakeHome, "probe.py")
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

	cmd := exec.Command(kiteBin, "exec", "--sandbox-profile=python-env", "--", pyBin, pyScript)
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
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	pyBin := findPython(t)

	kiteBin := buildKite(t)

	cmd := exec.Command(kiteBin, "exec", "--sandbox-profile=python-env", "--", pyBin, "-c", "import non_existent_starkite_module_xyz")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run() // Expect non-zero exit
	if sandboxUnavailable(stderr.String()) || (err != nil && sandboxUnavailable(err.Error())) {
		t.Skipf("host cannot start a sandbox: %s (err: %v)", stderr.String(), err)
	}

	wantHint := "Hint: pass --sandbox-profile=python-env on CLI, or add dependencies via --sandbox-path-read=<path>"
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

	cmd := exec.Command(kiteBin, "exec", "--sandbox-profile=node-env", "--", nodeBin, "-e", "require('non_existent_starkite_module_xyz')")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	wantHint := "Hint: pass --sandbox-profile=node-env on CLI, or grant access via --sandbox-path-read=node_modules"
	if !strings.Contains(stderr.String(), wantHint) {
		t.Errorf("expected node diagnostic hint in stderr, got:\n%s", stderr.String())
	}
}
