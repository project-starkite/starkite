package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"github.com/spf13/cobra"
)

func TestReplExit(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(".exit\n"))
	cmd.SetOut(&bytes.Buffer{})

	err := startRepl(cmd, nil)
	if err != nil {
		t.Fatalf("expected nil error on .exit, got: %v", err)
	}
}

func TestReplQuit(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(".quit\n"))
	cmd.SetOut(&bytes.Buffer{})

	err := startRepl(cmd, nil)
	if err != nil {
		t.Fatalf("expected nil error on .quit, got: %v", err)
	}
}

func TestReplEvalLines(t *testing.T) {
	cmd := &cobra.Command{}
	input := "x = 10\ny = 20\nz = x + y\n.exit\n"
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&bytes.Buffer{})

	err := startRepl(cmd, nil)
	if err != nil {
		t.Fatalf("expected nil error during evaluation, got: %v", err)
	}
}

func TestReplExitError(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("exit(3)\n"))
	cmd.SetOut(&bytes.Buffer{})

	err := startRepl(cmd, nil)
	if err == nil {
		t.Fatal("expected ExitError, got nil")
	}
	var exitErr *libkite.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *libkite.ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 3 {
		t.Fatalf("expected exit code 3, got %d", exitErr.Code)
	}
}

func TestReplMultiline(t *testing.T) {
	cmd := &cobra.Command{}
	input := "def add(a, b):\n    return a + b\n\nres = add(2, 3)\n.exit\n"
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&bytes.Buffer{})

	err := startRepl(cmd, nil)
	if err != nil {
		t.Fatalf("expected nil error in multiline execution, got: %v", err)
	}
}
