package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// ExecutionSpec defines the constraints and parameters for executing a command
// or script inside an isolated sandbox environment.
type ExecutionSpec struct {
	Command     []string      // Command and arguments (argv)
	Cwd         string        // Working directory for execution
	Env         []string      // KEY=VALUE environment strings
	Network     NetworkMode   // Network access mode (NetworkNone, NetworkLoopback, NetworkHost)
	Mounts      []Mount       // Filesystem mount bindings
	MaxMemoryMB int64         // Memory limit in megabytes (0 = unconstrained)
	MaxCPUs     float64       // CPU quota (0.0 = unconstrained)
	MaxPIDs     int64         // Maximum process count (0 = unconstrained)
	Timeout     time.Duration // Maximum execution duration (0 = unconstrained)
	Image       string        // Container image (for container/podman/docker drivers)
	Runtime     string        // Optional runtime override (e.g. "runsc")
	Stdin       io.Reader     // Standard input stream
	Stdout      io.Writer     // Standard output stream
	Stderr      io.Writer     // Standard error stream
}

// Validate checks the ExecutionSpec for basic invariants before execution.
func (s *ExecutionSpec) Validate() error {
	if len(s.Command) == 0 {
		return errors.New("sandbox: execution command cannot be empty")
	}
	if s.MaxMemoryMB < 0 {
		return fmt.Errorf("sandbox: max_memory cannot be negative: %d", s.MaxMemoryMB)
	}
	if s.MaxCPUs < 0 {
		return fmt.Errorf("sandbox: max_cpus cannot be negative: %f", s.MaxCPUs)
	}
	if s.MaxPIDs < 0 {
		return fmt.Errorf("sandbox: max_pids cannot be negative: %d", s.MaxPIDs)
	}
	if s.Timeout < 0 {
		return fmt.Errorf("sandbox: timeout cannot be negative: %v", s.Timeout)
	}
	for i, m := range s.Mounts {
		if m.Destination == "" {
			return fmt.Errorf("sandbox: mount[%d]: destination is required", i)
		}
		if m.Type == MountBind && m.Source == "" {
			return fmt.Errorf("sandbox: mount[%d]: bind mount requires source", i)
		}
	}
	return nil
}

// ExecResult contains the execution telemetry and output of a sandboxed process.
type ExecResult struct {
	ExitCode    int           // Process exit code
	Duration    time.Duration // Total elapsed execution time
	TimedOut    bool          // True if execution exceeded the timeout
	KilledByOOM bool          // True if process was terminated due to memory limits
	Stdout      string        // Captured standard output (if captured)
	Stderr      string        // Captured standard error (if captured)
}

// Driver is the core interface implemented by all sandbox isolation backends
// (e.g., Landlock on Linux, Seatbelt on macOS, Podman/Docker containers).
type Driver interface {
	// Name returns the unique identifier for the driver (e.g. "landlock", "seatbelt", "podman").
	Name() string

	// Available reports whether the driver is supported and usable on the current host.
	Available() bool

	// ValidateSpec verifies that the driver supports all constraints requested in the spec.
	ValidateSpec(spec *ExecutionSpec) error

	// Exec executes the command described by spec inside the sandbox environment.
	Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error)
}

// InProcessDriver is an optional interface implemented by native drivers that can
// apply sandbox restrictions directly to the running process (e.g., Landlock, Seatbelt).
type InProcessDriver interface {
	Driver
	ApplyInProcess(spec *ExecutionSpec) error
}
