//go:build lsp

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLSPCmdProbe(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	oldProbe := lspProbe
	lspProbe = true
	defer func() {
		lspProbe = oldProbe
	}()

	if err := runLSP(cmd, nil); err != nil {
		t.Fatalf("runLSP failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "namespaces") || !strings.Contains(out, "globals") {
		t.Errorf("expected probe output with namespaces and globals, got: %s", out)
	}
}
