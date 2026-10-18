package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestWatchScriptImmediateCancel(t *testing.T) {
	dir := t.TempDir()
	scriptFile := filepath.Join(dir, "target.star")
	if err := os.WriteFile(scriptFile, []byte("x = 42\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so watch loop exits right away

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	err := watchScript(cmd, []string{scriptFile})
	if err != nil {
		t.Fatalf("expected nil error on cancelled watch context, got: %v", err)
	}
}

func TestWatchScriptMissingTarget(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := watchScript(cmd, []string{"/nonexistent/target.star"})
	if err == nil {
		t.Fatal("expected error for missing watch target, got nil")
	}
}
