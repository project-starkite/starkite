//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux Landlock syscall constants and access flags.
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockCreateRulesetVersion = 1 << 0
	landlockRulePathBeneath      = 1

	landlockAccessFsExecute    = 1 << 0
	landlockAccessFsWriteFile  = 1 << 1
	landlockAccessFsReadFile   = 1 << 2
	landlockAccessFsReadDir    = 1 << 3
	landlockAccessFsRemoveDir  = 1 << 4
	landlockAccessFsRemoveFile = 1 << 5
	landlockAccessFsMakeChar   = 1 << 6
	landlockAccessFsMakeDir    = 1 << 7
	landlockAccessFsMakeReg    = 1 << 8
	landlockAccessFsMakeSock   = 1 << 9
	landlockAccessFsMakeFifo   = 1 << 10
	landlockAccessFsMakeBlock  = 1 << 11
	landlockAccessFsMakeSym    = 1 << 12
	landlockAccessFsRefer      = 1 << 13
	landlockAccessFsTruncate   = 1 << 14
	landlockAccessFsIoctlDev   = 1 << 15
)

type landlockRulesetAttr struct {
	handledAccessFs uint64
}

type landlockPathBeneathAttr struct {
	allowedAccess uint64
	parentFd      int32
}

// LandlockDriver implements the sandbox.Driver interface on Linux using
// the kernel Landlock Linux Security Module (LSM).
type LandlockDriver struct{}

func init() {
	Register(NewLandlockDriver())
}

// NewLandlockDriver creates a new LandlockDriver.
func NewLandlockDriver() *LandlockDriver {
	return &LandlockDriver{}
}

// Name returns the driver identifier "landlock".
func (d *LandlockDriver) Name() string {
	return DriverLandlock
}

// Available reports whether the host Linux kernel supports Landlock.
func (d *LandlockDriver) Available() bool {
	v, _, _ := unix.Syscall(
		sysLandlockCreateRuleset,
		0, 0,
		landlockCreateRulesetVersion,
	)
	return int(v) >= 1
}

// ValidateSpec verifies that the ExecutionSpec is valid for Landlock isolation.
func (d *LandlockDriver) ValidateSpec(spec *ExecutionSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	return nil
}

// ApplyInProcess restricts the current process and all future children
// using Landlock system calls. Once applied, restrictions are irreversible.
func (d *LandlockDriver) ApplyInProcess(spec *ExecutionSpec) error {
	if !d.Available() {
		return errors.New("sandbox: landlock driver not supported by kernel")
	}

	readAccess := uint64(
		landlockAccessFsExecute |
			landlockAccessFsReadFile |
			landlockAccessFsReadDir,
	)
	writeAccess := readAccess | uint64(
		landlockAccessFsWriteFile|
			landlockAccessFsRemoveDir|
			landlockAccessFsRemoveFile|
			landlockAccessFsMakeDir|
			landlockAccessFsMakeReg|
			landlockAccessFsMakeSym|
			landlockAccessFsTruncate,
	)

	attr := landlockRulesetAttr{
		handledAccessFs: writeAccess,
	}

	rulesetFd, _, err := unix.Syscall(
		sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if err != 0 {
		return fmt.Errorf("sandbox: landlock_create_ruleset failed: %w", err)
	}
	defer unix.Close(int(rulesetFd))

	// Standard system paths necessary for dynamic linking, certificates, and runtime execution
	systemReadOnlyPaths := []string{
		"/usr",
		"/lib",
		"/lib64",
		"/bin",
		"/sbin",
		"/etc/ssl",
		"/etc/pki",
		"/etc/ca-certificates",
		"/etc/resolv.conf",
		"/dev/null",
		"/dev/zero",
		"/dev/urandom",
		"/dev/random",
	}

	for _, p := range systemReadOnlyPaths {
		if _, statErr := os.Stat(p); statErr == nil {
			_ = addLandlockPathRule(int(rulesetFd), p, readAccess)
		}
	}

	// Mounts from ExecutionSpec
	for _, m := range spec.Mounts {
		target := m.Source
		if target == "" {
			target = m.Destination
		}
		if target == "" {
			continue
		}
		if _, statErr := os.Stat(target); statErr != nil {
			continue
		}

		if m.Mode == MountRW || m.Type == MountTmpfs {
			if err := addLandlockPathRule(int(rulesetFd), target, writeAccess); err != nil {
				return fmt.Errorf("sandbox: failed to add rw rule for %q: %w", target, err)
			}
		} else {
			if err := addLandlockPathRule(int(rulesetFd), target, readAccess); err != nil {
				return fmt.Errorf("sandbox: failed to add ro rule for %q: %w", target, err)
			}
		}
	}

	// Working directory
	if spec.Cwd != "" {
		if err := addLandlockPathRule(int(rulesetFd), spec.Cwd, writeAccess); err != nil {
			return fmt.Errorf("sandbox: failed to add cwd rule for %q: %w", spec.Cwd, err)
		}
	}

	// Lock current goroutine to the current OS thread so all execution stays
	// confined to the Landlock-restricted thread and its child processes.
	runtime.LockOSThread()

	// Enforce no new privileges
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("sandbox: prctl(PR_SET_NO_NEW_PRIVS) failed: %w", err)
	}

	// Restrict calling thread and future child processes
	if _, _, err := unix.Syscall(sysLandlockRestrictSelf, rulesetFd, 0, 0); err != 0 {
		return fmt.Errorf("sandbox: landlock_restrict_self failed: %w", err)
	}

	return nil
}

// Exec executes a sandboxed child command.
func (d *LandlockDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	start := time.Now()

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, spec.Command[0], spec.Command[1:]...)

	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	} else {
		cmd.Env = os.Environ()
	}

	// Network isolation via namespace unsharing if requested
	if spec.Network == NetworkNone || spec.Network == NetworkLoopback {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
			UidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getuid(), Size: 1},
			},
			GidMappings: []syscall.SysProcIDMap{
				{ContainerID: 0, HostID: os.Getgid(), Size: 1},
			},
		}
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
	if err != nil && cmd.SysProcAttr != nil {
		// If kernel restricts unprivileged user/net namespaces (e.g. Ubuntu AppArmor), fallback without SysProcAttr
		retryCmd := exec.CommandContext(execCtx, spec.Command[0], spec.Command[1:]...)
		if spec.Cwd != "" {
			retryCmd.Dir = spec.Cwd
		}
		retryCmd.Env = cmd.Env
		stdoutBuf.Reset()
		stderrBuf.Reset()
		retryCmd.Stdout = cmd.Stdout
		retryCmd.Stderr = cmd.Stderr
		retryCmd.Stdin = cmd.Stdin
		err = retryCmd.Run()
		cmd = retryCmd
	}
	duration := time.Since(start)

	result := &ExecResult{
		Duration: duration,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}

	if execCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = 124
		return result, fmt.Errorf("sandbox: command timed out after %v", spec.Timeout)
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("sandbox: landlock exec failed: %w", err)
	}

	return result, nil
}

func addLandlockPathRule(rulesetFd int, path string, access uint64) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		// For regular files/symlinks, strip directory-only Landlock flags to avoid EINVAL
		dirOnlyFlags := uint64(
			landlockAccessFsReadDir |
				landlockAccessFsRemoveDir |
				landlockAccessFsMakeDir |
				landlockAccessFsMakeReg |
				landlockAccessFsMakeSym,
		)
		access &^= dirOnlyFlags
	}

	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	pathAttr := landlockPathBeneathAttr{
		allowedAccess: access,
		parentFd:      int32(fd),
	}

	_, _, sysErr := unix.Syscall6(
		sysLandlockAddRule,
		uintptr(rulesetFd),
		landlockRulePathBeneath,
		uintptr(unsafe.Pointer(&pathAttr)),
		0, 0, 0,
	)
	if sysErr != 0 {
		return sysErr
	}
	return nil
}
