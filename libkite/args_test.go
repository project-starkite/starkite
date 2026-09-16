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

func TestEnsureScriptArgs(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}
	ctx1 := EnsureScriptArgs(thread)
	if ctx1 == nil {
		t.Fatal("expected non-nil context")
	}
	ctx2 := EnsureScriptArgs(thread)
	if ctx1 != ctx2 {
		t.Errorf("expected EnsureScriptArgs to return same context instance")
	}
	if EnsureScriptArgs(nil) == nil {
		t.Errorf("expected non-nil context for nil thread")
	}
}

func TestNormalizationHelpers(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		wantAttr string
		wantFlag string
	}{
		{name: "action", flag: "", wantAttr: "action", wantFlag: "action"},
		{name: "key-path", flag: "", wantAttr: "key_path", wantFlag: "key-path"},
		{name: "key_path", flag: "", wantAttr: "key_path", wantFlag: "key-path"},
		{name: "timeout", flag: "app-timeout", wantAttr: "timeout", wantFlag: "app-timeout"},
		{name: "timeout", flag: "--app-timeout", wantAttr: "timeout", wantFlag: "app-timeout"},
	}

	for _, tt := range tests {
		gotAttr := NormalizeAttributeName(tt.name)
		if gotAttr != tt.wantAttr {
			t.Errorf("NormalizeAttributeName(%q) = %q, want %q", tt.name, gotAttr, tt.wantAttr)
		}
		gotFlag := NormalizeFlagName(tt.name, tt.flag)
		if gotFlag != tt.wantFlag {
			t.Errorf("NormalizeFlagName(%q, %q) = %q, want %q", tt.name, tt.flag, gotFlag, tt.wantFlag)
		}
	}
}

func TestAddFlagValidation(t *testing.T) {
	ctx := &ScriptArgsContext{}

	// Valid flag
	err := ctx.AddFlag(FlagDef{
		Name:      "action",
		Flag:      "action",
		Type:      ArgTypeString,
		Shorthand: "a",
		Default:   starlark.String("install"),
	})
	if err != nil {
		t.Fatalf("unexpected error adding flag: %v", err)
	}

	// Duplicate flag name
	err = ctx.AddFlag(FlagDef{
		Name: "another_action",
		Flag: "action",
		Type: ArgTypeString,
	})
	if err == nil {
		t.Errorf("expected error for duplicate flag name, got nil")
	}

	// Duplicate attribute name
	err = ctx.AddFlag(FlagDef{
		Name: "action",
		Flag: "action-alias",
		Type: ArgTypeString,
	})
	if err == nil {
		t.Errorf("expected error for duplicate attribute name, got nil")
	}

	// Duplicate shorthand
	err = ctx.AddFlag(FlagDef{
		Name:      "all",
		Flag:      "all",
		Type:      ArgTypeBool,
		Shorthand: "a",
	})
	if err == nil {
		t.Errorf("expected error for duplicate shorthand, got nil")
	}

	// Multi-char shorthand
	err = ctx.AddFlag(FlagDef{
		Name:      "foo",
		Flag:      "foo",
		Type:      ArgTypeString,
		Shorthand: "bar",
	})
	if err == nil {
		t.Errorf("expected error for multi-char shorthand, got nil")
	}
}

func TestAddPositionalValidation(t *testing.T) {
	ctx := &ScriptArgsContext{}

	// Add required positional
	err := ctx.AddPositional(PositionalDef{
		Name:     "cluster-name",
		Attr:     "cluster_name",
		Required: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Add optional positional
	err = ctx.AddPositional(PositionalDef{
		Name:     "target-env",
		Attr:     "target_env",
		Required: false,
		Default:  starlark.String("dev"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cannot add required positional after optional
	err = ctx.AddPositional(PositionalDef{
		Name:     "config-file",
		Attr:     "config_file",
		Required: true,
	})
	if err == nil {
		t.Errorf("expected error when required positional follows optional, got nil")
	}

	// Duplicate positional name
	err = ctx.AddPositional(PositionalDef{
		Name:     "cluster-name",
		Attr:     "cluster_name_dup",
		Required: false,
	})
	if err == nil {
		t.Errorf("expected error for duplicate positional name, got nil")
	}
}
