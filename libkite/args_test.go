package libkite

import (
	"slices"
	"testing"

	"go.starlark.net/starlark"
)

func TestScriptArgsContext(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}

	// Initially nil
	if ctx := GetScriptArgs(thread); ctx != nil {
		t.Errorf("expected nil context initially, got %v", ctx)
	}

	// Set and retrieve
	expectedArgs := []string{"--action", "install", "my-cluster"}
	ctx := &ScriptArgsContext{
		RawArgs: expectedArgs,
		Parsed:  false,
	}
	SetScriptArgs(thread, ctx)

	retrieved := GetScriptArgs(thread)
	if retrieved == nil {
		t.Fatal("expected non-nil ScriptArgsContext")
	}
	if !slices.Equal(retrieved.RawArgs, expectedArgs) {
		t.Errorf("expected RawArgs %v, got %v", expectedArgs, retrieved.RawArgs)
	}
	if retrieved.Parsed {
		t.Errorf("expected Parsed=false initially")
	}

	// Mutate parsed latch
	retrieved.Parsed = true
	if !GetScriptArgs(thread).Parsed {
		t.Errorf("expected Parsed=true after mutation")
	}

	// Nil thread safety
	if ctx := GetScriptArgs(nil); ctx != nil {
		t.Errorf("expected nil for nil thread")
	}
	SetScriptArgs(nil, ctx) // should not panic
}
