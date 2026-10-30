package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var execCmd = &cobra.Command{
	Use:   "exec [--] <command> [args...]",
	Short: "Launch a native process or script in the sandbox",
	Long: `Launch an arbitrary external native process or executable script directly
under the Starkite kernel sandbox engine, completely bypassing the Starlark runtime.

Security Boundary:
  The kernel sandbox (Linux Landlock/Seccomp, macOS Seatbelt, Windows AppContainers)
  is authoritative across the entire child process hierarchy. Starlark language-level
  permissions do not apply to native machine code.

POSIX Flag Delimiter:
  Use '--' to cleanly separate Starkite runner flags from child arguments:
    kite exec --sandbox-net-allow -- curl -s https://example.com

Examples:
  # Launch a system tool under the default zero-trust sandbox
  kite exec /bin/echo "Hello from isolated sandbox"

  # Run a build script with working directory write and network egress
  kite exec --sandbox-path-write=. --sandbox-net-allow ./build.sh

  # Execute a binary in $PATH with child flags
  kite exec --sandbox-net-allow -- curl -I https://api.github.com
`,
	DisableFlagParsing: true,
	RunE:               runExecProcess,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

func runExecProcess(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		return cmd.Help()
	}

	targetCmd, childArgs, err := parseExecCommandLine(rootCmd.PersistentFlags(), args)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return cmd.Help()
		}
		return err
	}
	if targetCmd == "" {
		return &libkite.ScriptError{
			Message:  "missing required command target to execute",
			ExitCode: libkite.ExitUsageError,
		}
	}

	resolvedTarget, err := resolveExecTarget(targetCmd)
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitFileError,
		}
	}

	profile, err := ResolveActiveProfile()
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to resolve sandbox profile: %v", err),
			ExitCode: libkite.ExitConfigError,
		}
	}

	profName := profile.Name
	if profName == "" {
		profName = sandbox.DefaultProfileName
	}
	warnIfStarlarkPermissionFlagsPassed(profName)

	drvName := profile.Driver
	if drvName == "" || drvName == sandbox.DriverDefault || drvName == "auto" {
		d, err := sandbox.AutoDetect()
		if err != nil {
			return &libkite.ScriptError{
				Message:  fmt.Sprintf("failed to auto-detect sandbox driver: %v", err),
				ExitCode: libkite.ExitConfigError,
			}
		}
		drvName = d.Name()
	}

	driver, err := sandbox.Resolve(drvName)
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to resolve sandbox driver %q: %v", drvName, err),
			ExitCode: libkite.ExitConfigError,
		}
	}

	cwd, _ := os.Getwd()
	envSlice, err := profile.SynthesizeEnv(os.Environ(), envFlags, envInherit)
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to synthesize environment: %v", err),
			ExitCode: libkite.ExitConfigError,
		}
	}

	fullCommand := append([]string{resolvedTarget}, childArgs...)

	var execTimeout time.Duration
	if timeout > 0 {
		execTimeout = time.Duration(timeout) * time.Second
	} else {
		execTimeout = profile.Timeout
	}

	spec := &sandbox.ExecutionSpec{
		Command:          fullCommand,
		Cwd:              cwd,
		Env:              envSlice,
		Network:          profile.Network,
		NetworkAllow:     profile.NetworkAllow,
		NetworkDeny:      profile.NetworkDeny,
		Mounts:           profile.Mounts,
		MaxMemoryMB:      profile.MaxMemoryMB,
		Timeout:          execTimeout,
		Image:            profile.Image,
		ScriptFile:       resolvedTarget,
		NoExec:           false,
		AllowedExecPaths: profile.AllowedExecPaths,
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	}

	ctx, cancel := execContext(timeout)
	defer cancel()

	res, execErr := driver.Exec(ctx, spec)
	if execErr != nil {
		if res != nil && res.ExitCode > 0 {
			return &libkite.ExitError{Code: res.ExitCode}
		}
		return &libkite.ScriptError{
			Message:  execErr.Error(),
			ExitCode: libkite.ExitScriptError,
		}
	}

	if res != nil && res.ExitCode != 0 {
		return &libkite.ExitError{Code: res.ExitCode}
	}

	return nil
}

func parseExecCommandLine(flagSet *pflag.FlagSet, rawArgs []string) (target string, childArgs []string, err error) {
	var dashDashIdx = -1
	for i, a := range rawArgs {
		if a == "--" {
			dashDashIdx = i
			break
		}
	}

	if dashDashIdx != -1 {
		runnerArgs := rawArgs[:dashDashIdx]
		if len(runnerArgs) > 0 {
			if err := flagSet.Parse(runnerArgs); err != nil {
				if strings.Contains(err.Error(), "--sandbox-net") || strings.Contains(err.Error(), "unknown flag: --sandbox-net") {
					return "", nil, ErrSandboxNetRemoved
				}
				return "", nil, err
			}
		}
		child := rawArgs[dashDashIdx+1:]
		if len(child) == 0 {
			return "", nil, nil
		}
		return child[0], child[1:], nil
	}

	var targetIdx = -1
	for i := 0; i < len(rawArgs); i++ {
		arg := rawArgs[i]
		if !strings.HasPrefix(arg, "-") {
			targetIdx = i
			break
		}
		name := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
		if name == "sandbox-net" {
			return "", nil, ErrSandboxNetRemoved
		}
		f := flagSet.Lookup(name)
		if f == nil && len(name) == 1 {
			f = flagSet.ShorthandLookup(name)
		}
		if f != nil {
			isZeroArg := strings.Contains(arg, "=") || f.Value.Type() == "bool" || f.NoOptDefVal != ""
			if isZeroArg {
				if err := flagSet.Parse([]string{arg}); err != nil {
					return "", nil, err
				}
			} else if i+1 < len(rawArgs) {
				if err := flagSet.Parse([]string{arg, rawArgs[i+1]}); err != nil {
					return "", nil, err
				}
				i++
			} else {
				return "", nil, flagSet.Parse([]string{arg})
			}
			continue
		}
		if err := flagSet.Parse([]string{arg}); err != nil {
			return "", nil, err
		}
		targetIdx = i
		break
	}

	if targetIdx == -1 {
		return "", nil, nil
	}
	return rawArgs[targetIdx], rawArgs[targetIdx+1:], nil
}

func warnIfStarlarkPermissionFlagsPassed(profileName string) {
	if len(sandboxCallAllow) > 0 || len(sandboxCallDeny) > 0 {
		fmt.Fprintf(os.Stderr, "notice: target is a native process; Starlark permissions ignored. Applying kernel sandbox profile [%s].\n", profileName)
	}
}
