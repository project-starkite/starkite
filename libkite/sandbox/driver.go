package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ExecutionSpec defines the constraints and parameters for executing a command
// or script inside an isolated sandbox environment.
type ExecutionSpec struct {
	Command          []string      // Command and arguments (argv)
	Cwd              string        // Working directory for execution
	Env              []string      // KEY=VALUE environment strings
	Network          NetworkMode   // Network access mode (NetworkNone, NetworkLoopback, NetworkHost)
	Mounts           []Mount       // Filesystem mount bindings
	MaxMemoryMB      int64         // Memory limit in megabytes (0 = unconstrained)
	MaxCPUs          float64       // CPU quota (0.0 = unconstrained)
	MaxPIDs          int64         // Maximum process count (0 = unconstrained)
	Timeout          time.Duration // Maximum execution duration (0 = unconstrained)
	Image            string        // Container image (for container/podman/docker drivers)
	Runtime          string        // Optional runtime override (e.g. "runsc")
	ScriptFile       string        // Target script file being executed (for baseline protection)
	NoExec           bool          // Block subprocess execution (execve/execveat) inside the sandbox
	AllowedExecPaths []string      // Explicit allowlist of executable paths when restricting execution
	AuditLog         bool          // Log blocked syscalls via SECCOMP_RET_LOG instead of EPERM during audit testing
	Stdin            io.Reader     // Standard input stream
	Stdout           io.Writer     // Standard output stream
	Stderr           io.Writer     // Standard error stream
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
		if strings.ContainsAny(m.Destination, ":,\"\n\r\x00") || strings.ContainsAny(m.Source, ":,\"\n\r\x00") {
			return fmt.Errorf("sandbox: mount[%d]: source and destination must not contain ':', ',', quotes, newlines, or NUL bytes", i)
		}
	}
	for i, p := range s.AllowedExecPaths {
		if p == "" || !isHostAbs(p) {
			return fmt.Errorf("sandbox: allowed_exec_paths[%d] %q must be an absolute path", i, p)
		}
	}
	return nil
}

// FormatPermissionError wraps OS-level EACCES/EPERM errors with actionable
// sandbox policy attribution explaining why the operation was denied.
func FormatPermissionError(err error, targetPath string, spec *ExecutionSpec) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
		return err
	}
	if spec == nil {
		if targetPath != "" {
			return fmt.Errorf("sandbox: permission denied accessing %q: %w", targetPath, err)
		}
		return fmt.Errorf("sandbox: permission denied: %w", err)
	}
	if spec.NoExec && targetPath == "" {
		return fmt.Errorf("sandbox: subprocess execution denied by no_exec policy: %w", err)
	}
	if targetPath != "" {
		cleanTarget := filepath.ToSlash(filepath.Clean(targetPath))
		if cleanTarget == "/dev/shm" || strings.HasPrefix(cleanTarget, "/dev/shm/") ||
			cleanTarget == "/run/shm" || strings.HasPrefix(cleanTarget, "/run/shm/") {
			return fmt.Errorf("sandbox: access to %q denied by shared memory (/dev/shm) isolation: %w", targetPath, err)
		}
		if cleanTarget == "/tmp" || strings.HasPrefix(cleanTarget, "/tmp/") ||
			cleanTarget == "/private/tmp" || strings.HasPrefix(cleanTarget, "/private/tmp/") {
			return fmt.Errorf("sandbox: execution or access of %q denied on writable scratch mount (W^X noexec policy): %w", targetPath, err)
		}
		for _, m := range spec.Mounts {
			dst := filepath.ToSlash(filepath.Clean(m.Destination))
			src := filepath.ToSlash(filepath.Clean(m.Source))
			if (m.Mode == MountRW || m.Type == MountTmpfs) &&
				(cleanTarget == dst || strings.HasPrefix(cleanTarget, dst+"/") ||
					(m.Source != "" && (cleanTarget == src || strings.HasPrefix(cleanTarget, src+"/")))) {
				return fmt.Errorf("sandbox: execution of %q denied on writable mount %q (W^X noexec policy): %w", targetPath, m.Destination, err)
			}
		}
		if home, homeErr := os.UserHomeDir(); homeErr == nil && home != "" {
			cleanHome := filepath.ToSlash(filepath.Clean(home))
			if cleanTarget == cleanHome || strings.HasPrefix(cleanTarget, cleanHome+"/") {
				return fmt.Errorf("sandbox: access to %q denied by home directory isolation ($HOME is not mounted by default; add an explicit mount in your sandbox profile): %w", targetPath, err)
			}
		}
		if spec.NoExec {
			return fmt.Errorf("sandbox: execution of %q denied by no_exec policy: %w", targetPath, err)
		}
		if len(spec.AllowedExecPaths) > 0 {
			return fmt.Errorf("sandbox: execution of %q denied by allowed_exec_paths policy: %w", targetPath, err)
		}
		return fmt.Errorf("sandbox: access to %q denied by sandbox policy (path is outside active mounts or read-only): %w", targetPath, err)
	}
	return fmt.Errorf("sandbox: operation denied by sandbox policy: %w", err)
}

// enrichPermissionStderr appends actionable sandbox policy attribution from
// FormatPermissionError when a sandboxed process fails with a permission error.
func enrichPermissionStderr(stderr string, spec *ExecutionSpec) string {
	if spec == nil {
		return stderr
	}
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "sandbox:") {
		return stderr
	}
	if strings.Contains(lower, "unknown flag") ||
		strings.Contains(lower, "cannot connect to") ||
		strings.Contains(lower, "unable to connect to") {
		return stderr
	}
	if !strings.Contains(lower, "permission denied") &&
		!strings.Contains(lower, "operation not permitted") &&
		!strings.Contains(lower, "not permitted") &&
		!spec.NoExec && len(spec.AllowedExecPaths) == 0 {
		return stderr
	}

	target := ""
	for _, arg := range spec.Command {
		for tok := range strings.FieldsSeq(arg) {
			cleanTok := strings.Trim(tok, "'\"`;|&()")
			if strings.HasPrefix(cleanTok, "/") {
				target = cleanTok
				if cleanTok != "/bin/sh" && cleanTok != "/bin/bash" && cleanTok != "/usr/bin/env" {
					break
				}
			}
		}
	}
	diag := FormatPermissionError(syscall.EACCES, target, spec).Error()
	if stderr == "" {
		return diag + "\n"
	}
	if !strings.HasSuffix(stderr, "\n") {
		stderr += "\n"
	}
	return stderr + diag + "\n"
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
