package args_test

import (
	"context"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
)

func newTestRuntime(t *testing.T) *libkite.Runtime {
	t.Helper()
	reg := loader.NewDefaultRegistry(nil)
	rt, err := libkite.New(&libkite.Config{
		Registry: reg,
	})
	if err != nil {
		t.Fatalf("failed to create test runtime: %v", err)
	}
	return rt
}

func TestArgsString(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
		verify  func(t *testing.T, ctx *libkite.ScriptArgsContext)
	}{
		{
			name: "valid string flag",
			script: `
args.string("action", flag="cluster-action", shorthand="a", default="install", required=True, choices=["install", "upgrade"], help="Action to run", var_fallback="ACTION")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				if len(ctx.Flags) != 1 {
					t.Fatalf("expected 1 flag, got %d", len(ctx.Flags))
				}
				f := ctx.Flags[0]
				if f.Name != "action" || f.Flag != "cluster-action" || f.Shorthand != "a" {
					t.Errorf("unexpected flag properties: %+v", f)
				}
				if f.Type != libkite.ArgTypeString {
					t.Errorf("expected ArgTypeString, got %v", f.Type)
				}
				if !f.Required {
					t.Errorf("expected Required=true")
				}
				if len(f.Choices) != 2 || f.Choices[0] != "install" {
					t.Errorf("unexpected choices: %v", f.Choices)
				}
				if f.Help != "Action to run" || f.VarFallback != "ACTION" {
					t.Errorf("unexpected help or var_fallback: %+v", f)
				}
				if f.Default != starlark.String("install") {
					t.Errorf("unexpected default: %v", f.Default)
				}
			},
		},
		{
			name: "hyphenated name auto-normalizes",
			script: `
args.string("key-path", shorthand="k")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				f := ctx.Flags[0]
				if f.Name != "key_path" {
					t.Errorf("expected normalized Name='key_path', got %q", f.Name)
				}
				if f.Flag != "key-path" {
					t.Errorf("expected Flag='key-path', got %q", f.Flag)
				}
			},
		},
		{
			name:    "empty name error",
			script:  `args.string("")`,
			wantErr: "name cannot be empty",
		},
		{
			name:    "default not in choices",
			script:  `args.string("action", default="destroy", choices=["install", "upgrade"])`,
			wantErr: "default value \"destroy\" is not in allowed choices",
		},
		{
			name:    "invalid choices item type",
			script:  `args.string("action", choices=[1, 2])`,
			wantErr: "choices items must be strings",
		},
		{
			name:    "invalid default type",
			script:  `args.string("action", default=123)`,
			wantErr: "default value must be a string",
		},
		{
			name:    "invalid shorthand length",
			script:  `args.string("action", shorthand="act")`,
			wantErr: "must be a single character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				ctx := libkite.GetScriptArgs(rt.Thread())
				tt.verify(t, ctx)
			}
		})
	}
}

func TestArgsInt(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
		verify  func(t *testing.T, ctx *libkite.ScriptArgsContext)
	}{
		{
			name: "valid int flag with min max",
			script: `
args.int("replicas", shorthand="r", default=3, min=1, max=10, required=True, help="Number of replicas")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				if len(ctx.Flags) != 1 {
					t.Fatalf("expected 1 flag, got %d", len(ctx.Flags))
				}
				f := ctx.Flags[0]
				if f.Name != "replicas" || f.Flag != "replicas" || f.Shorthand != "r" {
					t.Errorf("unexpected flag properties: %+v", f)
				}
				if f.Type != libkite.ArgTypeInt {
					t.Errorf("expected ArgTypeInt, got %v", f.Type)
				}
				if f.Min == nil || *f.Min != 1.0 {
					t.Errorf("expected min=1, got %v", f.Min)
				}
				if f.Max == nil || *f.Max != 10.0 {
					t.Errorf("expected max=10, got %v", f.Max)
				}
				if f.Default != starlark.MakeInt(3) {
					t.Errorf("unexpected default: %v", f.Default)
				}
			},
		},
		{
			name:    "min greater than max",
			script:  `args.int("count", min=10, max=5)`,
			wantErr: "min (10) cannot be greater than max (5)",
		},
		{
			name:    "default less than min",
			script:  `args.int("count", default=2, min=5)`,
			wantErr: "default value (2) cannot be less than min (5)",
		},
		{
			name:    "default greater than max",
			script:  `args.int("count", default=20, max=10)`,
			wantErr: "default value (20) cannot be greater than max (10)",
		},
		{
			name:    "invalid default type",
			script:  `args.int("count", default="three")`,
			wantErr: "default must be an int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				ctx := libkite.GetScriptArgs(rt.Thread())
				tt.verify(t, ctx)
			}
		})
	}
}

func TestArgsBool(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
		verify  func(t *testing.T, ctx *libkite.ScriptArgsContext)
	}{
		{
			name: "valid bool flag with default",
			script: `
args.bool("dry-run", shorthand="d", default=False, help="Simulate execution")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				if len(ctx.Flags) != 1 {
					t.Fatalf("expected 1 flag, got %d", len(ctx.Flags))
				}
				f := ctx.Flags[0]
				if f.Name != "dry_run" || f.Flag != "dry-run" || f.Shorthand != "d" {
					t.Errorf("unexpected flag properties: %+v", f)
				}
				if f.Type != libkite.ArgTypeBool {
					t.Errorf("expected ArgTypeBool, got %v", f.Type)
				}
				if f.Default != starlark.Bool(false) {
					t.Errorf("expected default=False, got %v", f.Default)
				}
			},
		},
		{
			name:    "invalid default type",
			script:  `args.bool("verbose", default="yes")`,
			wantErr: "default value must be a bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				ctx := libkite.GetScriptArgs(rt.Thread())
				tt.verify(t, ctx)
			}
		})
	}
}

func TestArgsFloat(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
		verify  func(t *testing.T, ctx *libkite.ScriptArgsContext)
	}{
		{
			name: "valid float flag with bounds",
			script: `
args.float("ratio", default=0.75, min=0.0, max=1.0, help="Ratio threshold")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				if len(ctx.Flags) != 1 {
					t.Fatalf("expected 1 flag, got %d", len(ctx.Flags))
				}
				f := ctx.Flags[0]
				if f.Name != "ratio" || f.Type != libkite.ArgTypeFloat {
					t.Errorf("unexpected flag properties: %+v", f)
				}
				if f.Min == nil || *f.Min != 0.0 || f.Max == nil || *f.Max != 1.0 {
					t.Errorf("unexpected min/max: min=%v, max=%v", f.Min, f.Max)
				}
			},
		},
		{
			name:    "float min greater than max",
			script:  `args.float("rate", min=2.5, max=1.0)`,
			wantErr: "min (2.5) cannot be greater than max (1)",
		},
		{
			name:    "default out of float bounds",
			script:  `args.float("rate", default=3.5, max=2.0)`,
			wantErr: "default value (3.5) cannot be greater than max (2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				ctx := libkite.GetScriptArgs(rt.Thread())
				tt.verify(t, ctx)
			}
		})
	}
}

func TestArgsList(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
		verify  func(t *testing.T, ctx *libkite.ScriptArgsContext)
	}{
		{
			name: "valid list of strings",
			script: `
args.list("workers", shorthand="w", default=["node-1", "node-2"], item_type="string", help="Worker nodes")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				if len(ctx.Flags) != 1 {
					t.Fatalf("expected 1 flag, got %d", len(ctx.Flags))
				}
				f := ctx.Flags[0]
				if f.Name != "workers" || f.Type != libkite.ArgTypeList || f.ItemType != "string" {
					t.Errorf("unexpected flag properties: %+v", f)
				}
			},
		},
		{
			name:    "invalid item_type",
			script:  `args.list("items", item_type="dict")`,
			wantErr: "invalid item_type \"dict\"",
		},
		{
			name:    "default item does not match item_type",
			script:  `args.list("ports", default=[80, "invalid"], item_type="int")`,
			wantErr: "default list element must be int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				ctx := libkite.GetScriptArgs(rt.Thread())
				tt.verify(t, ctx)
			}
		})
	}
}

func TestArgsPositional(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
		verify  func(t *testing.T, ctx *libkite.ScriptArgsContext)
	}{
		{
			name: "valid positionals required then optional",
			script: `
args.positional("cluster-name", help="Cluster identifier")
args.positional("manifest", default="deploy.yaml", help="Manifest path")
`,
			verify: func(t *testing.T, ctx *libkite.ScriptArgsContext) {
				if len(ctx.Positionals) != 2 {
					t.Fatalf("expected 2 positionals, got %d", len(ctx.Positionals))
				}
				p1 := ctx.Positionals[0]
				if p1.Name != "cluster-name" || p1.Attr != "cluster_name" || !p1.Required {
					t.Errorf("unexpected positional 1: %+v", p1)
				}
				p2 := ctx.Positionals[1]
				if p2.Name != "manifest" || p2.Attr != "manifest" || p2.Required {
					t.Errorf("unexpected positional 2: %+v", p2)
				}
				if p2.Default != starlark.String("deploy.yaml") {
					t.Errorf("unexpected default on positional 2: %v", p2.Default)
				}
			},
		},
		{
			name: "required positional cannot follow optional",
			script: `
args.positional("target", default="local")
args.positional("required-arg", required=True)
`,
			wantErr: "required positional argument \"required-arg\" cannot follow optional positional argument \"target\"",
		},
		{
			name:    "positional cannot be both required and have default",
			script:  `args.positional("target", required=True, default="local")`,
			wantErr: "cannot be both required and have a default value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				ctx := libkite.GetScriptArgs(rt.Thread())
				tt.verify(t, ctx)
			}
		})
	}
}

func TestArgsCollisions(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr string
	}{
		{
			name: "duplicate flag name",
			script: `
args.string("action")
args.string("other", flag="action")
`,
			wantErr: "flag \"action\" is already defined",
		},
		{
			name: "duplicate shorthand",
			script: `
args.string("action", shorthand="a")
args.string("all", shorthand="a")
`,
			wantErr: "shorthand -a is already defined",
		},
		{
			name: "positional conflicts with flag attribute",
			script: `
args.string("target-name")
args.positional("target_name")
`,
			wantErr: "conflicts with an existing argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTestRuntime(t)
			err := rt.Execute(context.Background(), tt.script)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestArgsParseStub(t *testing.T) {
	rt := newTestRuntime(t)
	script := `
args.string("action", default="install")
args.positional("cluster-name")
p = args.parse()
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected error executing script: %v", err)
	}
	ctx := libkite.GetScriptArgs(rt.Thread())
	if !ctx.Parsed {
		t.Errorf("expected ctx.Parsed=true after args.parse()")
	}
}
