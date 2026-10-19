package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveExecutablePaths_SymlinksAndBareNames(t *testing.T) {
	tempDir := t.TempDir()

	realDir := filepath.Join(tempDir, "real")
	sym1Dir := filepath.Join(tempDir, "sym1")
	sym2Dir := filepath.Join(tempDir, "sym2")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sym1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sym2Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	realBin := filepath.Join(realDir, "mytool")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	sym1Bin := filepath.Join(sym1Dir, "mytool")
	if err := os.Symlink(realBin, sym1Bin); err != nil {
		t.Fatal(err)
	}

	sym2Bin := filepath.Join(sym2Dir, "mytool")
	if err := os.Symlink(sym1Bin, sym2Bin); err != nil {
		t.Fatal(err)
	}

	// 1. Direct symlink traversal
	resolved := ResolveExecutablePaths([]string{sym2Bin}, tempDir, "")
	cleanReal, _ := filepath.EvalSymlinks(realBin)
	cleanSym1, _ := filepath.EvalSymlinks(sym1Bin)
	cleanSym2, _ := filepath.Clean(sym2Bin), filepath.Clean(sym1Bin)
	_ = cleanSym2

	if !slices.Contains(resolved, filepath.Clean(sym2Bin)) {
		t.Errorf("expected resolved paths to contain sym2Bin (%q), got: %v", sym2Bin, resolved)
	}
	if !slices.Contains(resolved, cleanSym1) && !slices.Contains(resolved, filepath.Clean(sym1Bin)) {
		t.Errorf("expected resolved paths to contain sym1Bin, got: %v", resolved)
	}
	if !slices.Contains(resolved, cleanReal) {
		t.Errorf("expected resolved paths to contain real target (%q), got: %v", cleanReal, resolved)
	}

	// 2. Comma-separated entries and PATH resolution
	t.Setenv("PATH", sym2Dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	resolvedBare := ResolveExecutablePaths([]string{"mytool,nonexistent-tool", "*"}, tempDir, "")
	if !slices.Contains(resolvedBare, cleanReal) {
		t.Errorf("expected bare binary lookup to resolve real target (%q), got: %v", cleanReal, resolvedBare)
	}
	if slices.Contains(resolvedBare, "*") {
		t.Errorf("wildcard * should not appear in resolved paths, got: %v", resolvedBare)
	}
}
