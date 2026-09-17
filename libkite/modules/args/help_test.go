package args_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	argsmod "github.com/project-starkite/starkite/libkite/modules/args"
)

func TestFormatScriptHelp(t *testing.T) {
	ctx := &libkite.ScriptArgsContext{
		ScriptPath: "./deploy.star",
	}

	_ = ctx.AddFlag(libkite.FlagDef{
		Name:      "action",
		Flag:      "action",
		Type:      libkite.ArgTypeString,
		Shorthand: "a",
		Default:   starlark.String("install"),
		Choices:   []string{"install", "upgrade"},
		Help:      "Deployment action",
	})
	_ = ctx.AddFlag(libkite.FlagDef{
		Name:      "replicas",
		Flag:      "replicas",
		Type:      libkite.ArgTypeInt,
		Shorthand: "r",
		Default:   starlark.MakeInt(3),
		Help:      "Replica count",
	})
	_ = ctx.AddFlag(libkite.FlagDef{
		Name:      "dry_run",
		Flag:      "dry-run",
		Type:      libkite.ArgTypeBool,
		Shorthand: "d",
		Help:      "Simulate execution",
	})
	_ = ctx.AddPositional(libkite.PositionalDef{
		Name:     "cluster-name",
		Attr:     "cluster_name",
		Required: true,
		Help:     "Target cluster identifier",
	})
	_ = ctx.AddPositional(libkite.PositionalDef{
		Name:     "manifest",
		Attr:     "manifest",
		Required: false,
		Default:  starlark.String("deploy.yaml"),
		Help:     "Path to manifest",
	})

	help := argsmod.FormatScriptHelp(ctx)

	if !strings.Contains(help, "Usage: kite run ./deploy.star [flags] <cluster-name> [manifest]") {
		t.Errorf("expected usage line, got:\n%s", help)
	}
	if !strings.Contains(help, "Arguments:") {
		t.Errorf("expected Arguments header, got:\n%s", help)
	}
	if !strings.Contains(help, "<cluster-name>") || !strings.Contains(help, "(required)") {
		t.Errorf("expected required positional in help, got:\n%s", help)
	}
	if !strings.Contains(help, "[manifest]") || !strings.Contains(help, `(default: "deploy.yaml")`) {
		t.Errorf("expected optional positional with default in help, got:\n%s", help)
	}
	if !strings.Contains(help, "Flags:") {
		t.Errorf("expected Flags header, got:\n%s", help)
	}
	if !strings.Contains(help, "-a, --action string") || !strings.Contains(help, "(choices: install, upgrade)") {
		t.Errorf("expected action flag with choices, got:\n%s", help)
	}
	if !strings.Contains(help, "-r, --replicas int") || !strings.Contains(help, "(default: 3)") {
		t.Errorf("expected replicas flag with default, got:\n%s", help)
	}
	if !strings.Contains(help, "-d, --dry-run") {
		t.Errorf("expected dry-run flag, got:\n%s", help)
	}
	if !strings.Contains(help, "-h, --help") || !strings.Contains(help, "Show help for deploy.star") {
		t.Errorf("expected standard help flag, got:\n%s", help)
	}
}

func TestScriptHelpExecution(t *testing.T) {
	var printedOutput bytes.Buffer

	reg := libkite.NewRegistry(nil)
	reg.Register(argsmod.New())

	cfg := &libkite.Config{
		Registry:   reg,
		ScriptPath: "./app.star",
		ScriptArgs: []string{"--help"},
		Print: func(_ *starlark.Thread, msg string) {
			printedOutput.WriteString(msg + "\n")
		},
	}

	rt, err := libkite.New(cfg)
	if err != nil {
		t.Fatalf("libkite.New error: %v", err)
	}

	script := `
args.string("env", default="staging", help="Target environment")
args.positional("app-name", required=True, help="Application name")

p = args.parse()
`
	err = rt.Execute(context.Background(), script)
	if err != nil {
		t.Fatalf("expected clean exit (nil error) on --help, got: %v", err)
	}

	ctx := libkite.GetScriptArgs(rt.Thread())
	if !ctx.Parsed {
		t.Errorf("expected ctx.Parsed=true on --help")
	}

	out := printedOutput.String()
	if !strings.Contains(out, "Usage: kite run ./app.star [flags] <app-name>") {
		t.Errorf("expected usage in output, got:\n%s", out)
	}
	if !strings.Contains(out, "--env string") || !strings.Contains(out, "(default: \"staging\")") {
		t.Errorf("expected env flag in output, got:\n%s", out)
	}
	if !strings.Contains(out, "-h, --help") {
		t.Errorf("expected -h, --help in output, got:\n%s", out)
	}
}

func TestScriptShorthandHelpExecution(t *testing.T) {
	var printedOutput bytes.Buffer

	reg := libkite.NewRegistry(nil)
	reg.Register(argsmod.New())

	cfg := &libkite.Config{
		Registry:   reg,
		ScriptPath: "./service.star",
		ScriptArgs: []string{"-h"},
		Print: func(_ *starlark.Thread, msg string) {
			printedOutput.WriteString(msg + "\n")
		},
	}

	rt, err := libkite.New(cfg)
	if err != nil {
		t.Fatalf("libkite.New error: %v", err)
	}

	script := `
args.string("tier", default="frontend")
p = args.parse()
`
	err = rt.Execute(context.Background(), script)
	if err != nil {
		t.Fatalf("expected clean exit on -h, got: %v", err)
	}

	out := printedOutput.String()
	if !strings.Contains(out, "Usage: kite run ./service.star") {
		t.Errorf("expected usage in output, got:\n%s", out)
	}
}
