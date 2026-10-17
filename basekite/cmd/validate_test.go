package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/syntax"
)

func TestParseStarlarkErrorsWindowsPath(t *testing.T) {
	// Fallback regex with Windows drive letter
	winErr := fmt.Errorf("C:\\projects\\app\\deploy.star:15:8: undefined: invalid_var")
	errs := parseStarlarkErrors("deploy.star", winErr)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].File != "C:\\projects\\app\\deploy.star" || errs[0].Line != 15 || errs[0].Column != 8 || errs[0].Message != "undefined: invalid_var" {
		t.Fatalf("unexpected error parsing Windows path: %+v", errs[0])
	}

	// syntax.Error direct extraction
	pos := syntax.MakePosition(nil, 42, 10)
	synErr := syntax.Error{Pos: pos, Msg: "got unexpected token"}
	errs = parseStarlarkErrors("test.star", synErr)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error from syntax.Error, got %d", len(errs))
	}
	if errs[0].File != "test.star" || errs[0].Line != 42 || errs[0].Column != 10 || errs[0].Message != "got unexpected token" {
		t.Fatalf("unexpected error parsing syntax.Error: %+v", errs[0])
	}
}

func TestValidateExitCodes(t *testing.T) {
	dir := t.TempDir()

	validFile := filepath.Join(dir, "valid.star")
	if err := os.WriteFile(validFile, []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	brokenFile := filepath.Join(dir, "broken.star")
	if err := os.WriteFile(brokenFile, []byte("x = ((\n"), 0644); err != nil {
		t.Fatal(err)
	}

	missingFile := filepath.Join(dir, "nonexistent.star")

	// Valid file -> nil error
	if err := runValidate(nil, []string{validFile}); err != nil {
		t.Fatalf("expected nil error for valid file, got: %v", err)
	}

	// Broken file -> ScriptError with ExitScriptError (1)
	err := runValidate(nil, []string{brokenFile})
	if err == nil {
		t.Fatal("expected error for syntax error file, got nil")
	}
	scriptErr, ok := errors.AsType[*libkite.ScriptError](err)
	if !ok {
		t.Fatalf("expected *libkite.ScriptError, got %T: %v", err, err)
	}
	if scriptErr.ExitCode != libkite.ExitScriptError {
		t.Fatalf("expected exit code %d (ExitScriptError), got %d", libkite.ExitScriptError, scriptErr.ExitCode)
	}

	// Missing file -> ScriptError with ExitFileError (2)
	err = runValidate(nil, []string{missingFile})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	scriptErr, ok = errors.AsType[*libkite.ScriptError](err)
	if !ok {
		t.Fatalf("expected *libkite.ScriptError, got %T: %v", err, err)
	}
	if scriptErr.ExitCode != libkite.ExitFileError {
		t.Fatalf("expected exit code %d (ExitFileError), got %d", libkite.ExitFileError, scriptErr.ExitCode)
	}
}

func TestWithinDirEdgeCases(t *testing.T) {
	dir := t.TempDir()

	// Child directory
	sub := filepath.Join(dir, "sub")
	if !withinDir(dir, sub) {
		t.Fatalf("withinDir(%q, %q) = false, want true", dir, sub)
	}

	// Same directory
	if !withinDir(dir, dir) {
		t.Fatalf("withinDir(%q, %q) = false, want true", dir, dir)
	}

	// Filename starting with .. inside dir (e.g. dir/..foo)
	dotDotFile := filepath.Join(dir, "..foo")
	if !withinDir(dir, dotDotFile) {
		t.Fatalf("withinDir(%q, %q) = false, want true for ..foo inside dir", dir, dotDotFile)
	}

	// Parent directory escape
	parentDir := filepath.Dir(dir)
	if withinDir(dir, parentDir) {
		t.Fatalf("withinDir(%q, %q) = true, want false for parent", dir, parentDir)
	}
}
