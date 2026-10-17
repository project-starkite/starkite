package osmod

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/project-starkite/starkite/libkite"
)

func TestRunCommandTimeout_NoStdoutBufferRace(t *testing.T) {
	reg := libkite.NewRegistry(&libkite.ModuleConfig{})
	reg.Register(New())
	rt, err := libkite.NewTrusted(nil, libkite.WithRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Repeatedly trigger a command timeout while the child process actively writes
	// to stdout and stderr. Under -race this verifies that syncBuffer and waiting
	// for cmd.Wait() prevent any data race on stdout/stderr buffers.
	for range 5 {
		err := rt.Execute(context.Background(), `
os.sh().try_exec("while true; do echo stdout_line; echo stderr_line >&2; done", timeout="20ms")
`)
		if err != nil {
			t.Fatalf("unexpected error with try_exec: %v", err)
		}
	}
}

func TestRunCommand_ContextCancellation(t *testing.T) {
	reg := libkite.NewRegistry(&libkite.ModuleConfig{})
	reg.Register(New())
	rt, err := libkite.NewTrusted(nil, libkite.WithRegistry(reg))
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = rt.Execute(ctx, `os.exec("sleep 10")`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected canceled/deadline error, got: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("os.exec took %v to cancel, expected < 2s", elapsed)
	}
}
