//go:build lsp

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestKitelspProbe(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	probeFlag = true
	defer func() { probeFlag = false }()

	if err := runServer(rootCmd, nil); err != nil {
		t.Fatalf("runServer failed: %v", err)
	}

	out := buf.String()
	for _, expected := range []string{"namespaces", "globals", "fs", "http", "k8s"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected probe output to contain %q, got:\n%s", expected, out)
		}
	}
}
