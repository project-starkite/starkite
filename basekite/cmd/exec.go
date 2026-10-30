package cmd

import (
	"errors"
	"fmt"
	"io"
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

	stderrBuf := newBoundedBuffer(maxStderrDiagnosticCap)
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
		Stderr:           io.MultiWriter(os.Stderr, stderrBuf),
	}

	ctx, cancel := execContext(timeout)
	defer cancel()

	res, execErr := driver.Exec(ctx, spec)
	if res != nil {
		if res.Stderr == "" && stderrBuf.Len() > 0 {
			res.Stderr = stderrBuf.String()
		}
		prevLen := len(res.Stderr)
		sandbox.EnrichExecDiagnostics(res, spec)
		if len(res.Stderr) > prevLen {
			_, _ = io.WriteString(os.Stderr, res.Stderr[prevLen:])
		}
		if debugMode {
			fmt.Fprintf(os.Stderr, "[telemetry] command=%q exit_code=%d duration=%v timed_out=%v driver=%s\n",
				strings.Join(spec.Command, " "), res.ExitCode, res.Duration, res.TimedOut, driver.Name())
		}
	} else if debugMode {
		fmt.Fprintf(os.Stderr, "[telemetry] command=%q exit_code=%d duration=%v timed_out=%v driver=%s\n",
			strings.Join(spec.Command, " "), 1, 0, false, driver.Name())
	}
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

const maxStderrDiagnosticCap = 2 * 1024 * 1024 // 2MB

// boundedBuffer is an io.Writer that retains at most maxCap bytes in memory,
// discarding the oldest bytes when capacity is exceeded to prevent unbounded
// memory growth during diagnostic inspection of long-running or verbose processes.
type boundedBuffer struct {
	buf    []byte
	maxCap int
}

func newBoundedBuffer(maxCap int) *boundedBuffer {
	return &boundedBuffer{
		buf:    make([]byte, 0, min(maxCap, 64*1024)),
		maxCap: maxCap,
	}
}

func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	n = len(p)
	if b.maxCap <= 0 {
		return n, nil
	}
	if len(p) >= b.maxCap {
		b.buf = append(b.buf[:0], p[len(p)-b.maxCap:]...)
		return n, nil
	}
	overflow := (len(b.buf) + len(p)) - b.maxCap
	if overflow > 0 {
		copy(b.buf, b.buf[overflow:])
		b.buf = b.buf[:len(b.buf)-overflow]
	}
	b.buf = append(b.buf, p...)
	return n, nil
}

func (b *boundedBuffer) String() string {
	return string(b.buf)
}

func (b *boundedBuffer) Len() int {
	return len(b.buf)
}
