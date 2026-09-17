package cmd

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	baseloader "github.com/project-starkite/starkite/libkite/loader"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.starlark.net/starlark"
)

func TestParseKnownFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var vars []string
	fs.StringArrayVar(&vars, "var", nil, "variables")
	dryRun := fs.Bool("dry-run", false, "dry run")
	output := fs.StringP("output", "o", "text", "output")
	allowFs := fs.String("allow-fs", "", "allow fs")
	fs.Lookup("allow-fs").NoOptDefVal = "true"

	input := []string{
		"--var", "env=prod",
		"--var=replicas=5",
		"--dry-run",
		"-o", "json",
		"--allow-fs",
		"./deploy.star",
		"--action", "install",
		"--replicas", "3",
		"my-cluster",
	}

	target, scriptArgs, err := parseScriptCommandLine(fs, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target != "./deploy.star" {
		t.Errorf("expected target ./deploy.star, got %s", target)
	}
	expectedVars := []string{"env=prod", "replicas=5"}
	if !slices.Equal(vars, expectedVars) {
		t.Errorf("expected vars %v, got %v", expectedVars, vars)
	}
	if !*dryRun {
		t.Errorf("expected dryRun=true")
	}
	if *output != "json" {
		t.Errorf("expected output=json, got %s", *output)
	}
	if *allowFs != "true" {
		t.Errorf("expected allowFs=true, got %s", *allowFs)
	}

	expectedScriptArgs := []string{"--action", "install", "--replicas", "3", "my-cluster"}
	if !slices.Equal(scriptArgs, expectedScriptArgs) {
		t.Errorf("expected scriptArgs %v, got %v", expectedScriptArgs, scriptArgs)
	}
}

func TestRunCommandExecutionParsing(t *testing.T) {
	root := &cobra.Command{Use: "kite"}
	var timeoutVal int
	var dryRunVal bool
	var varsVal []string

	root.PersistentFlags().IntVar(&timeoutVal, "timeout", 300, "timeout")
	root.PersistentFlags().BoolVar(&dryRunVal, "dry-run", false, "dry run")
	root.PersistentFlags().StringArrayVar(&varsVal, "var", nil, "variables")

	var capturedTarget string
	var capturedScriptArgs []string

	run := &cobra.Command{
		Use:                "run <target>",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
				return cmd.Help()
			}
			target, scriptArgs, err := parseScriptCommandLine(root.PersistentFlags(), args)
			if err != nil {
				return err
			}
			capturedTarget = target
			capturedScriptArgs = scriptArgs
			return nil
		},
	}
	root.AddCommand(run)

	// Test 1: kite run --help
	{
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetArgs([]string{"run", "--help"})
		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error on --help: %v", err)
		}
		if !strings.Contains(buf.String(), "Usage:") {
			t.Errorf("expected help output, got: %s", buf.String())
		}
	}

	// Test 2: kite run script.star --action install my-cluster
	{
		capturedTarget = ""
		capturedScriptArgs = nil
		root.SetArgs([]string{"run", "script.star", "--action", "install", "my-cluster"})
		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedTarget != "script.star" {
			t.Errorf("expected target script.star, got %s", capturedTarget)
		}
		expectedArgs := []string{"--action", "install", "my-cluster"}
		if !slices.Equal(capturedScriptArgs, expectedArgs) {
			t.Errorf("expected scriptArgs %v, got %v", expectedArgs, capturedScriptArgs)
		}
	}

	// Test 3: kite run --timeout 15 script.star --action install -r 3
	{
		timeoutVal = 300
		capturedTarget = ""
		capturedScriptArgs = nil
		root.SetArgs([]string{"run", "--timeout", "15", "script.star", "--action", "install", "-r", "3"})
		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if timeoutVal != 15 {
			t.Errorf("expected timeoutVal 15, got %d", timeoutVal)
		}
		if capturedTarget != "script.star" {
			t.Errorf("expected target script.star, got %s", capturedTarget)
		}
		expectedArgs := []string{"--action", "install", "-r", "3"}
		if !slices.Equal(capturedScriptArgs, expectedArgs) {
			t.Errorf("expected scriptArgs %v, got %v", expectedArgs, capturedScriptArgs)
		}
	}

	// Test 4: kite run --timeout 30 script.star -- --timeout 5s --dry-run my-cluster
	{
		timeoutVal = 300
		dryRunVal = false
		capturedTarget = ""
		capturedScriptArgs = nil
		root.SetArgs([]string{"run", "--timeout", "30", "script.star", "--", "--timeout", "5s", "--dry-run", "my-cluster"})
		if err := root.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if timeoutVal != 30 {
			t.Errorf("expected timeoutVal 30 on kite, got %d", timeoutVal)
		}
		if dryRunVal != false {
			t.Errorf("expected dryRunVal false on kite, got %v", dryRunVal)
		}
		if capturedTarget != "script.star" {
			t.Errorf("expected target script.star, got %s", capturedTarget)
		}
		expectedArgs := []string{"--timeout", "5s", "--dry-run", "my-cluster"}
		if !slices.Equal(capturedScriptArgs, expectedArgs) {
			t.Errorf("expected scriptArgs %v, got %v", expectedArgs, capturedScriptArgs)
		}
	}
}

func TestScriptArgsIntegrationWithRuntime(t *testing.T) {
	expectedArgs := []string{"--action", "deploy", "--replicas", "3", "prod-cluster"}
	var receivedArgs []string

	checkArgsBuiltin := starlark.NewBuiltin("check_args", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		ctx := libkite.GetScriptArgs(thread)
		if ctx != nil {
			receivedArgs = ctx.RawArgs
		}
		return starlark.None, nil
	})

	cfg := &libkite.Config{
		ScriptArgs: expectedArgs,
		Globals: map[string]any{
			"check_args": checkArgsBuiltin,
		},
	}

	rt, err := libkite.New(cfg)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer rt.Cleanup()

	script := `check_args()`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	if !slices.Equal(receivedArgs, expectedArgs) {
		t.Errorf("expected receivedArgs %v, got %v", expectedArgs, receivedArgs)
	}
}

func TestScriptArgsEndToEndWithArgsModule(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Bool("dry-run", false, "dry run")

	// CLI input: kite run ./deploy.star --action upgrade -r 4 prod-cluster
	input := []string{"./deploy.star", "--action", "upgrade", "-r", "4", "prod-cluster"}
	target, scriptArgs, err := parseScriptCommandLine(fs, input)
	if err != nil {
		t.Fatalf("parseScriptCommandLine error: %v", err)
	}
	if target != "./deploy.star" {
		t.Fatalf("unexpected target: %s", target)
	}

	reg := baseloader.NewDefaultRegistry(nil)
	cfg := &libkite.Config{
		Registry:   reg,
		ScriptArgs: scriptArgs,
	}

	rt, err := libkite.New(cfg)
	if err != nil {
		t.Fatalf("libkite.New error: %v", err)
	}
	defer rt.Cleanup()

	script := `
args.string("action", default="install")
args.int("replicas", shorthand="r", default=1)
args.positional("cluster")

p = args.parse()

assert(p.action == "upgrade", "action should be upgrade")
assert(p.replicas == 4, "replicas should be 4")
assert(p.cluster == "prod-cluster", "cluster should be prod-cluster")
`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestUnhandledArgumentsStrictCheck(t *testing.T) {
	// Script that ignores arguments and does not call args.parse()
	cfg := &libkite.Config{
		ScriptPath: "./task.star",
		ScriptArgs: []string{"--action", "install"},
	}

	rt, err := libkite.New(cfg)
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer rt.Cleanup()

	script := `print("doing simple task without args.parse")`
	if err := rt.Execute(context.Background(), script); err != nil {
		t.Fatalf("unexpected execute failure: %v", err)
	}

	// Verify post-run strict check
	sCtx := libkite.GetScriptArgs(rt.Thread())
	if sCtx == nil {
		t.Fatal("expected non-nil script context")
	}
	if len(sCtx.RawArgs) == 0 || sCtx.Parsed {
		t.Fatalf("expected unparsed raw args in context")
	}

	// Test simulated runScript check
	var scriptErr error
	if len(sCtx.RawArgs) > 0 && !sCtx.Parsed {
		scriptErr = &libkite.ScriptError{
			Message:  "unhandled arguments: " + strings.Join(sCtx.RawArgs, " "),
			ExitCode: libkite.ExitUsageError,
		}
	}

	if scriptErr == nil {
		t.Fatalf("expected scriptErr for unhandled arguments, got nil")
	}
	sErr, ok := scriptErr.(*libkite.ScriptError)
	if !ok || sErr.ExitCode != libkite.ExitUsageError {
		t.Errorf("expected ExitUsageError (%d), got %v", libkite.ExitUsageError, scriptErr)
	}
}

func TestScriptHelpRoutingAfterTarget(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.BoolP("help", "h", false, "help")
	fs.Bool("dry-run", false, "dry run")

	// Invocations where --help or -h is AFTER target:
	target1, scriptArgs1, err := parseScriptCommandLine(fs, []string{"./deploy.star", "--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target1 != "./deploy.star" || len(scriptArgs1) != 1 || scriptArgs1[0] != "--help" {
		t.Errorf("expected scriptArgs=['--help'], got target=%q, args=%v", target1, scriptArgs1)
	}

	target2, scriptArgs2, err := parseScriptCommandLine(fs, []string{"--dry-run", "./deploy.star", "-h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target2 != "./deploy.star" || len(scriptArgs2) != 1 || scriptArgs2[0] != "-h" {
		t.Errorf("expected scriptArgs=['-h'], got target=%q, args=%v", target2, scriptArgs2)
	}
}
