package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

func TestExecCodeSuccess(t *testing.T) {
	// Simple arithmetic and assignment
	err := execCode(nil, []string{"x = 1 + 2"})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestExecCodeExitError(t *testing.T) {
	err := execCode(nil, []string{"exit(7)"})
	if err == nil {
		t.Fatal("expected ExitError, got nil")
	}
	var exitErr *libkite.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *libkite.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 7 {
		t.Fatalf("expected exit code 7, got %d", exitErr.Code)
	}
}

func TestExecCodeSyntaxError(t *testing.T) {
	err := execCode(nil, []string{"x = (("})
	if err == nil {
		t.Fatal("expected error on syntax error, got nil")
	}
	var scriptErr *libkite.ScriptError
	if !errors.As(err, &scriptErr) {
		t.Fatalf("expected *libkite.ScriptError, got %T: %v", err, err)
	}
	if scriptErr.ExitCode != libkite.ExitScriptError {
		t.Fatalf("expected ExitScriptError, got %d", scriptErr.ExitCode)
	}
}

func TestExecCodePermissionsDeny(t *testing.T) {
	oldEnabled := sandboxEnabled
	oldProfile := sandboxProfile
	sandboxEnabled = true
	sandboxProfile = "deny-all"
	defer func() {
		sandboxEnabled = oldEnabled
		sandboxProfile = oldProfile
	}()

	err := execCode(nil, []string{"exec('echo test')"})
	if err == nil {
		t.Fatal("expected permission denied error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected 'permission denied' in error, got: %v", err)
	}
}

func TestExecCodeWithVars(t *testing.T) {
	oldVars := variables
	variables = []string{"greeting=hello_starkite"}
	defer func() { variables = oldVars }()

	err := execCode(nil, []string{"assert(var_str('greeting') == 'hello_starkite')"})
	if err != nil {
		t.Fatalf("expected successful execution with var flag, got: %v", err)
	}
}
