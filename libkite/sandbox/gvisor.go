package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Standard gVisor standalone runtime binary name.
const (
	BinRunsc = "runsc"
)

// GVisorDriver implements the Driver interface for gVisor sandboxing.
// It integrates with gVisor as an external OCI provider:
// 1. Delegating via Podman/Docker with --runtime=runsc (preferred in container environments).
// 2. Invoking standalone /usr/bin/runsc directly in rootless mode on Linux hosts.
type GVisorDriver struct {
	mu        sync.Mutex
	runscPath string
}

func init() {
	Register(NewGVisorDriver())
}

// NewGVisorDriver creates a new GVisorDriver and probes for runtime support.
func NewGVisorDriver() *GVisorDriver {
	bin, _ := exec.LookPath(BinRunsc)
	return &GVisorDriver{
		runscPath: bin,
	}
}

// Name returns the driver identifier "gvisor".
func (d *GVisorDriver) Name() string {
	return DriverGVisor
}

func (d *GVisorDriver) resolveRunsc() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.runscPath != "" {
		if _, err := os.Stat(d.runscPath); err == nil {
			return d.runscPath
		}
		d.runscPath = ""
	}
	if bin, err := exec.LookPath(BinRunsc); err == nil {
		d.runscPath = bin
		return bin
	}
	return ""
}

// Available reports whether standalone runsc is installed on this host.
func (d *GVisorDriver) Available() bool {
	return d.resolveRunsc() != ""
}

// ValidateSpec verifies that the ExecutionSpec is valid for gVisor execution.
func (d *GVisorDriver) ValidateSpec(spec *ExecutionSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if !d.Available() {
		return errors.New("sandbox: gVisor runtime (runsc or container engine with runsc) not found")
	}
	return nil
}

// Exec executes the command inside a gVisor sandboxed environment.
func (d *GVisorDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	// Strategy A: Delegate through available container engine with --runtime=runsc
	for _, engineName := range []string{DriverPodman, DriverDocker, DriverNerdctl} {
		if drv, err := Get(engineName); err == nil && drv.Available() {
			containerSpec := *spec
			containerSpec.Runtime = "runsc"
			return drv.Exec(ctx, &containerSpec)
		}
	}

	// Strategy B: Direct standalone runsc execution (Linux)
	if d.resolveRunsc() != "" {
		return d.execStandaloneRunsc(ctx, spec)
	}

	return nil, errors.New("sandbox: no usable gVisor execution backend found")
}

func (d *GVisorDriver) execStandaloneRunsc(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if policyErr := checkContainerExecPolicy(spec); policyErr != nil {
		return policyDeniedResult(spec, policyErr), nil
	}
	if spec.Network != NetworkHost && spec.Network != "" {
		return nil, fmt.Errorf("sandbox: standalone runsc cannot enforce network=%q without a container engine (podman/docker/nerdctl)", spec.Network)
	}
	if len(spec.Mounts) > 0 || spec.MaxMemoryMB > 0 || spec.MaxCPUs > 0 || spec.MaxPIDs > 0 || spec.NoExec || len(spec.AllowedExecPaths) > 0 {
		return nil, errors.New("sandbox: standalone runsc cannot enforce mounts, resource limits, or exec restrictions without a container engine (podman/docker/nerdctl)")
	}
	runscPath := d.resolveRunsc()
	if runscPath == "" {
		return nil, errors.New("sandbox: standalone runsc binary not found")
	}

	start := time.Now()

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	args := []string{"--rootless", "do", "--"}
	args = append(args, spec.Command...)

	cmd := exec.CommandContext(execCtx, runscPath, args...)
	configureSubprocessCancel(cmd)

	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	} else {
		cmd.Env = mapToEnvSlice(SanitizedBaselineEnv(os.Environ()))
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	if spec.Stdout != nil {
		cmd.Stdout = spec.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	if spec.Stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderrBuf, spec.Stderr)
	} else {
		cmd.Stderr = &stderrBuf
	}

	if spec.Stdin != nil {
		cmd.Stdin = spec.Stdin
	}

	err := cmd.Run()
	duration := time.Since(start)

	if execCtx.Err() != nil {
		reapSubprocessGroup(cmd)
	}

	result := &ExecResult{
		Duration: duration,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}

	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.ExitCode = 124
		if spec.Timeout > 0 {
			return result, fmt.Errorf("sandbox: gVisor command timed out after %v: %w", spec.Timeout, execCtx.Err())
		}
		return result, fmt.Errorf("sandbox: gVisor command timed out: %w", execCtx.Err())
	}
	if errors.Is(execCtx.Err(), context.Canceled) {
		if cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}
		return result, execCtx.Err()
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			result.ExitCode = exitErr.ExitCode()
			applyPermissionStderr(result, spec)
			return result, nil
		}
		return result, FormatPermissionError(fmt.Errorf("sandbox: gVisor runsc exec failed: %w", err), spec.Command[0], spec)
	}

	applyPermissionStderr(result, spec)

	return result, nil
}
