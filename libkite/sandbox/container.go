package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Standard container engine names.
const (
	EnginePodman  = "podman"
	EngineDocker  = "docker"
	EngineNerdctl = "nerdctl"

	// DefaultContainerImage is the fallback base image if none is specified in ExecutionSpec.
	DefaultContainerImage = "docker.io/library/alpine:latest"
)

// ContainerDriver implements the Driver interface by orchestrating ephemeral
// containers via the Podman, Docker, or Nerdctl CLI.
type ContainerDriver struct {
	engine  string // "podman", "docker", or "nerdctl"
	binPath string // Path to the container CLI binary
}

func init() {
	Register(NewPodmanDriver())
	Register(NewDockerDriver())
	Register(NewNerdctlDriver())
}

// NewPodmanDriver creates a new ContainerDriver configured for Podman.
func NewPodmanDriver() *ContainerDriver {
	bin, _ := exec.LookPath(EnginePodman)
	return &ContainerDriver{
		engine:  EnginePodman,
		binPath: bin,
	}
}

// NewDockerDriver creates a new ContainerDriver configured for Docker.
func NewDockerDriver() *ContainerDriver {
	bin, _ := exec.LookPath(EngineDocker)
	return &ContainerDriver{
		engine:  EngineDocker,
		binPath: bin,
	}
}

// NewNerdctlDriver creates a new ContainerDriver configured for Nerdctl.
func NewNerdctlDriver() *ContainerDriver {
	bin, _ := exec.LookPath(EngineNerdctl)
	return &ContainerDriver{
		engine:  EngineNerdctl,
		binPath: bin,
	}
}

// NewContainerDriver creates a ContainerDriver with a specific engine name and binary path.
func NewContainerDriver(engine, binPath string) *ContainerDriver {
	return &ContainerDriver{
		engine:  engine,
		binPath: binPath,
	}
}

// Name returns the driver identifier ("podman" or "docker").
func (d *ContainerDriver) Name() string {
	return d.engine
}

// Available reports whether the container CLI binary is installed and executable on the host.
func (d *ContainerDriver) Available() bool {
	if d.binPath == "" {
		d.binPath, _ = exec.LookPath(d.engine)
	}
	return d.binPath != ""
}

// ValidateSpec verifies that the ExecutionSpec is valid for containerized execution.
func (d *ContainerDriver) ValidateSpec(spec *ExecutionSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if !d.Available() {
		return fmt.Errorf("sandbox: container engine %q CLI binary not found in PATH", d.engine)
	}
	return nil
}

// BuildArgs translates an ExecutionSpec into standard OCI container CLI arguments.
func (d *ContainerDriver) BuildArgs(spec *ExecutionSpec) []string {
	args := []string{"run", "--rm", "-i"}

	// Network configuration
	switch spec.Network {
	case NetworkHost:
		args = append(args, "--network=host")
	case NetworkNone, NetworkLoopback, "":
		args = append(args, "--network=none")
	}

	// Working directory
	if spec.Cwd != "" {
		args = append(args, "--workdir", spec.Cwd)
	}

	// Environment variables
	for _, env := range spec.Env {
		args = append(args, "-e", env)
	}

	// Resource limits
	if spec.MaxMemoryMB > 0 {
		args = append(args, fmt.Sprintf("--memory=%dm", spec.MaxMemoryMB))
	}
	if spec.MaxCPUs > 0 {
		args = append(args, fmt.Sprintf("--cpus=%f", spec.MaxCPUs))
	}
	if spec.MaxPIDs > 0 {
		args = append(args, fmt.Sprintf("--pids-limit=%d", spec.MaxPIDs))
	}

	// Mount bindings
	for _, m := range spec.Mounts {
		if m.Type == MountTmpfs {
			args = append(args, fmt.Sprintf("--tmpfs=%s:rw,noexec,nosuid", m.Destination))
			continue
		}

		src := m.Source
		if src == "" {
			src = m.Destination
		}

		mode := "ro"
		if m.Mode == MountRW {
			mode = "rw"
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", src, m.Destination, mode))
	}

	// Ensure executable binary is accessible inside container if it is an absolute host path
	if len(spec.Command) > 0 && filepath.IsAbs(spec.Command[0]) {
		binPath := spec.Command[0]
		if _, err := os.Stat(binPath); err == nil {
			isMounted := false
			for _, m := range spec.Mounts {
				if m.Source == binPath || (m.Source != "" && strings.HasPrefix(binPath, m.Source)) {
					isMounted = true
					break
				}
			}
			if !isMounted {
				args = append(args, "-v", fmt.Sprintf("%s:%s:ro", binPath, binPath))
			}
		}
	}

	// Custom runtime override (e.g. --runtime=runsc)
	if spec.Runtime != "" {
		args = append(args, fmt.Sprintf("--runtime=%s", spec.Runtime))
	}

	// Container Image
	image := spec.Image
	if image == "" {
		image = DefaultContainerImage
	}
	args = append(args, image)

	// Command and argv
	args = append(args, spec.Command...)

	return args
}

// Exec executes the command inside an ephemeral container using the container engine CLI.
func (d *ContainerDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	start := time.Now()
	cliArgs := d.BuildArgs(spec)

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, d.binPath, cliArgs...)

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
		return result, fmt.Errorf("sandbox: container command timed out after %v", spec.Timeout)
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			// Check for standard OOM exit codes (137 = 128 + SIGKILL)
			if result.ExitCode == 137 && strings.Contains(strings.ToLower(result.Stderr), "oom") {
				result.KilledByOOM = true
			}
			return result, nil
		}
		return result, fmt.Errorf("sandbox: container execution failed: %w", err)
	}

	return result, nil
}
