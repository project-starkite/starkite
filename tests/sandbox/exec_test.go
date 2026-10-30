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

func TestExec_EchoStdoutStreaming(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	kiteBin := buildKite(t)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(kiteBin, "exec", "--", "cmd.exe", "/c", "echo starkite-native-exec-verification")
	} else {
		cmd = exec.Command(kiteBin, "exec", "--", "echo", "starkite-native-exec-verification")
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if sandboxUnavailable(stderr.String()) {
			t.Skipf("host cannot start a sandbox; skipping: %s", stderr.String())
		}
		t.Fatalf("kite exec failed: %v\nStderr: %s", err, stderr.String())
	}

	outStr := strings.TrimSpace(stdout.String())
	if outStr != "starkite-native-exec-verification" {
		t.Errorf("unexpected stdout output: %q", outStr)
	}
}

func TestExec_ExitCodePropagation(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	kiteBin := buildKite(t)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(kiteBin, "exec", "--", "cmd.exe", "/c", "exit 42")
	} else {
		tmpDir := createNonHomeTempDir(t)
		script := filepath.Join(tmpDir, "exit42.sh")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 42\n"), 0755); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(kiteBin, "exec", script)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit, got nil")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v\nStderr: %s", err, err, stderr.String())
	}
	if exitErr.ExitCode() != 42 {
		t.Errorf("expected exit code 42, got %d\nStderr: %s", exitErr.ExitCode(), stderr.String())
	}
}

func TestExec_NetworkConfinement(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available on host")
	}
	kiteBin := buildKite(t)

	cmdDeny := exec.Command(kiteBin, "exec", "--sandbox-profile=deny-all", "--", "curl", "-s", "--connect-timeout", "2", "https://example.com")
	if err := cmdDeny.Run(); err == nil {
		t.Error("expected curl to fail under deny-all network profile, but succeeded")
	}

	cmdAllow := exec.Command(kiteBin, "exec", "--sandbox-net-allow", "--", "curl", "-s", "--connect-timeout", "5", "https://example.com")
	if err := cmdAllow.Run(); err != nil {
		t.Logf("curl with net allow failed (possible offline test env): %v", err)
	}
}
