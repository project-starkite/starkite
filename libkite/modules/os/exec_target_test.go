package osmod

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/project-starkite/starkite/libkite"
)

func TestResolveExecTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path semantics differ on windows")
	}

	t.Run("absolute path is returned cleaned", func(t *testing.T) {
		got := resolveExecTarget("/usr/bin/kubectl apply -f x", "", nil)
		want := libkite.CanonicalizePath("/usr/bin/kubectl")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("relative path resolves against workDir", func(t *testing.T) {
		got := resolveExecTarget("./build.sh --all", "/home/alice/proj", nil)
		want := libkite.CanonicalizePath("/home/alice/proj/build.sh")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("bare name resolves on PATH override", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "mytool")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		got := resolveExecTarget("mytool --version", "", map[string]string{"PATH": dir})
		want := libkite.CanonicalizePath(bin)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("unresolvable bare name falls back to the token", func(t *testing.T) {
		got := resolveExecTarget("definitely-not-a-real-binary-xyz arg", "", map[string]string{"PATH": "/nonexistent"})
		if got != "definitely-not-a-real-binary-xyz" {
			t.Errorf("got %q, want the bare token", got)
		}
	})

	t.Run("empty command returns input", func(t *testing.T) {
		if got := resolveExecTarget("", "", nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
