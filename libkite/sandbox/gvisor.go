package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// Available reports whether gVisor is accessible on this host (via container engines or standalone runsc).
func (d *GVisorDriver) Available() bool {
	// 1. Check if standalone runsc binary exists in PATH
	if d.runscPath != "" {
		return true
	}
	if bin, err := exec.LookPath(BinRunsc); err == nil {
		d.runscPath = bin
		return true
	}

	// 2. Check if a container engine (podman, docker, nerdctl) is available to host runsc
	for _, engineName := range []string{DriverPodman, DriverDocker, DriverNerdctl} {
		if drv, err := Get(engineName); err == nil && drv.Available() {
			return true
		}
	}

	return false
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
	if d.runscPath != "" {
		return d.execStandaloneRunsc(ctx, spec)
	}

	return nil, errors.New("sandbox: no usable gVisor execution backend found")
}

func (d *GVisorDriver) execStandaloneRunsc(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	start := time.Now()

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	args := []string{"--rootless", "do", "--"}
	args = append(args, spec.Command...)

	cmd := exec.CommandContext(execCtx, d.runscPath, args...)

	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	} else {
		cmd.Env = os.Environ()
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	if spec.Stdout != nil {
		cmd.Stdout = spec.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	if spec.Stderr != nil {
		cmd.Stderr = spec.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	if spec.Stdin != nil {
		cmd.Stdin = spec.Stdin
	}

	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecResult{
		Duration: duration,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}

	if execCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = 124
		return result, fmt.Errorf("sandbox: gVisor command timed out after %v", spec.Timeout)
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("sandbox: gVisor runsc exec failed: %w", err)
	}

	return result, nil
}
