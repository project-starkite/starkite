package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestExec_CommandLineParsing(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var sandboxed bool
	var netAllow []string
	flags.BoolVar(&sandboxed, "sandbox-enabled", false, "")
	flags.StringArrayVar(&netAllow, "sandbox-net-allow", nil, "")
	flags.Lookup("sandbox-net-allow").NoOptDefVal = "*"

	t.Run("with dash-dash delimiter", func(t *testing.T) {
		args := []string{"--sandbox-enabled", "--sandbox-net-allow", "--", "curl", "-s", "https://example.com"}
		target, child, err := parseExecCommandLine(flags, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target != "curl" {
			t.Errorf("expected target 'curl', got %q", target)
		}
		if len(child) != 2 || child[0] != "-s" || child[1] != "https://example.com" {
			t.Errorf("unexpected child args: %v", child)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true")
		}
		if len(netAllow) == 0 || netAllow[0] != "*" {
			t.Errorf("expected netAllow=['*'], got %v", netAllow)
		}
	})

	t.Run("without dash-dash delimiter", func(t *testing.T) {
		sandboxed = false
		netAllow = nil
		args := []string{"--sandbox-enabled", "echo", "hello", "world"}
		target, child, err := parseExecCommandLine(flags, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target != "echo" {
			t.Errorf("expected target 'echo', got %q", target)
		}
		if len(child) != 2 || child[0] != "hello" || child[1] != "world" {
			t.Errorf("unexpected child args: %v", child)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true")
		}
	})

	t.Run("intercepts --sandbox-net with dash-dash", func(t *testing.T) {
		args := []string{"--sandbox-net", "--", "curl"}
		_, _, err := parseExecCommandLine(flags, args)
		if !errors.Is(err, ErrSandboxNetRemoved) {
			t.Errorf("expected ErrSandboxNetRemoved, got: %v", err)
		}
	})

	t.Run("intercepts --sandbox-net without dash-dash", func(t *testing.T) {
		args := []string{"--sandbox-net", "curl"}
		_, _, err := parseExecCommandLine(flags, args)
		if !errors.Is(err, ErrSandboxNetRemoved) {
			t.Errorf("expected ErrSandboxNetRemoved, got: %v", err)
		}
	})

	t.Run("rejects unknown runner flag without dash-dash", func(t *testing.T) {
		args := []string{"--unknown-runner-flag", "echo"}
		_, _, err := parseExecCommandLine(flags, args)
		if err == nil || !strings.Contains(err.Error(), "unknown flag: --unknown-runner-flag") {
			t.Errorf("expected unknown flag error, got: %v", err)
		}
	})
}

func TestExec_WarnStarlarkPermissionFlags(t *testing.T) {
	oldAllow := sandboxCallAllow
	defer func() { sandboxCallAllow = oldAllow }()
	sandboxCallAllow = []string{"fs.read"}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	os.Stderr = w

	warnIfStarlarkPermissionFlagsPassed("custom-profile")
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	out := buf.String()
	want := "notice: target is a native process; Starlark permissions ignored. Applying kernel sandbox profile [custom-profile].\n"
	if out != want {
		t.Errorf("unexpected notice output: got %q, want %q", out, want)
	}
}

func TestExec_TargetResolution(t *testing.T) {
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "submodule")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	_, err := resolveExecTarget(subDir)
	if err == nil || !strings.Contains(err.Error(), "is a directory; use 'kite run'") {
		t.Errorf("expected directory rejection error, got: %v", err)
	}

	if runtime.GOOS != "windows" {
		nonExecFile := filepath.Join(tmpDir, "script.sh")
		if err := os.WriteFile(nonExecFile, []byte("#!/bin/sh\necho hi\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err = resolveExecTarget(nonExecFile)
		if err == nil || !strings.Contains(err.Error(), "not executable (missing chmod +x)") {
			t.Errorf("expected non-executable rejection error, got: %v", err)
		}

		if err := os.Chmod(nonExecFile, 0755); err != nil {
			t.Fatal(err)
		}
		resolved, err := resolveExecTarget(nonExecFile)
		if err != nil {
			t.Fatalf("expected successful resolution, got: %v", err)
		}
		if !filepath.IsAbs(resolved) {
			t.Errorf("expected absolute path, got: %s", resolved)
		}
	}

	lookupTarget := "echo"
	if runtime.GOOS == "windows" {
		lookupTarget = "cmd"
	}
	resolvedTarget, err := resolveExecTarget(lookupTarget)
	if err != nil {
		t.Fatalf("expected '%s' in PATH, got: %v", lookupTarget, err)
	}
	if resolvedTarget == "" {
		t.Errorf("expected non-empty path for %s", lookupTarget)
	}

	_, err = resolveExecTarget("nonexistent_binary_xyz_123")
	if err == nil || !strings.Contains(err.Error(), "not found in working directory or $PATH") {
		t.Errorf("expected missing binary error, got: %v", err)
	}
}

func TestExec_RunGuardrail_NonStarlarkScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission test")
	}
	tmpDir := t.TempDir()
	shScript := filepath.Join(tmpDir, "deploy.sh")
	if err := os.WriteFile(shScript, []byte("#!/bin/sh\necho deploy\n"), 0755); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveRunTarget(shScript)
	if err == nil {
		t.Fatal("expected error running shell script with 'kite run', got nil")
	}
	if !strings.Contains(err.Error(), "is not a Starkite script or module") || !strings.Contains(err.Error(), "Use 'kite exec") {
		t.Errorf("unexpected guardrail error message: %v", err)
	}
}
