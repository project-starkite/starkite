//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	// minWindowsBuild24H2 is Windows 11 Version 24H2 / Windows Server 2025 (Build 26100).
	minWindowsBuild24H2 = 26100
)

// WindowsDriver implements the sandbox.Driver interface on Windows using
// Windows Job Objects and STARTUPINFOEXW process mitigations.
type WindowsDriver struct{}

func init() {
	Register(NewWindowsDriver())
}

// NewWindowsDriver creates a new WindowsDriver instance.
func NewWindowsDriver() *WindowsDriver {
	return &WindowsDriver{}
}

// Name returns the driver identifier "windows".
func (d *WindowsDriver) Name() string {
	return DriverWindows
}

// Available reports whether the host Windows operating system meets the minimum
// kernel baseline for native sandboxing.
func (d *WindowsDriver) Available() bool {
	return checkWindowsOSBaseline() == nil
}

func checkWindowsOSBaseline() error {
	if os.Getenv("STARKITE_WINDOWS_ALLOW_LEGACY_BUILD") == "1" {
		return nil
	}
	major, _, build := windows.RtlGetNtVersionNumbers()
	if major < 10 || build < minWindowsBuild24H2 {
		return fmt.Errorf(
			"sandbox: host environment does not support required Windows kernel sandbox primitives: Windows build %d < %d (requires Windows 11 24H2+ or Windows Server 2025+)",
			build, minWindowsBuild24H2,
		)
	}
	return nil
}

// ValidateSpec verifies that the ExecutionSpec is valid and supported on Windows.
func (d *WindowsDriver) ValidateSpec(spec *ExecutionSpec) error {
	if spec == nil {
		return fmt.Errorf("sandbox: execution spec cannot be nil")
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	if err := checkWindowsOSBaseline(); err != nil {
		return err
	}
	return nil
}

// Exec executes the command described by spec inside a Windows Job Object and
// STARTUPINFOEXW mitigation boundary.
func (d *WindowsDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	exePath, err := resolveWindowsCommandPath(spec.Command[0], spec.Cwd)
	if err != nil {
		return nil, fmt.Errorf("sandbox: failed to resolve executable %q: %w", spec.Command[0], err)
	}

	job, err := newWindowsJob(spec)
	if err != nil {
		return nil, err
	}
	defer job.close()

	// Set up inheritable standard I/O pipes.
	stdinR, stdinW, err := createInheritablePipe(true)
	if err != nil {
		return nil, err
	}
	defer func() {
		if stdinR != nil {
			_ = stdinR.Close()
		}
		if stdinW != nil {
			_ = stdinW.Close()
		}
	}()

	stdoutR, stdoutW, err := createInheritablePipe(false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if stdoutR != nil {
			_ = stdoutR.Close()
		}
		if stdoutW != nil {
			_ = stdoutW.Close()
		}
	}()

	stderrR, stderrW, err := createInheritablePipe(false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if stderrR != nil {
			_ = stderrR.Close()
		}
		if stderrW != nil {
			_ = stderrW.Close()
		}
	}()

	stdinHandle := windows.Handle(stdinR.Fd())
	stdoutHandle := windows.Handle(stdoutW.Fd())
	stderrHandle := windows.Handle(stderrW.Fd())

	attrs, err := buildWindowsStartupAttrs(spec, stdinHandle, stdoutHandle, stderrHandle)
	if err != nil {
		return nil, err
	}
	defer attrs.close()

	cmdLine, err := buildWindowsCommandLine(exePath, spec.Command[1:])
	if err != nil {
		return nil, err
	}
	appNamePtr, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return nil, err
	}

	var cwdPtr *uint16
	if spec.Cwd != "" {
		cwdPtr, err = windows.UTF16PtrFromString(spec.Cwd)
		if err != nil {
			return nil, err
		}
	}

	envVars := spec.Env
	if len(envVars) == 0 {
		baseMap := SanitizedBaselineEnv(os.Environ())
		envVars = make([]string, 0, len(baseMap))
		for k, v := range baseMap {
			envVars = append(envVars, k+"="+v)
		}
	}
	envBlock, err := createWindowsEnvBlock(envVars)
	if err != nil {
		return nil, err
	}
	var envPtr *uint16
	if len(envBlock) > 0 {
		envPtr = &envBlock[0]
	}

	var procInfo windows.ProcessInformation
	creationFlags := uint32(windows.CREATE_SUSPENDED | windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)

	startTime := time.Now()
	err = windows.CreateProcess(
		appNamePtr,
		cmdLine,
		nil,
		nil,
		true, // bInheritHandles (restricted via PROC_THREAD_ATTRIBUTE_HANDLE_LIST)
		creationFlags,
		envPtr,
		cwdPtr,
		&attrs.startupInfoEx.StartupInfo,
		&procInfo,
	)
	if err != nil {
		return nil, fmt.Errorf("sandbox: CreateProcessW (%s) failed: %w", exePath, err)
	}
	defer func() {
		_ = windows.CloseHandle(procInfo.Thread)
		_ = windows.CloseHandle(procInfo.Process)
	}()

	// Close child-side pipe ends in parent immediately after CreateProcess.
	_ = stdinR.Close()
	stdinR = nil
	_ = stdoutW.Close()
	stdoutW = nil
	_ = stderrW.Close()
	stderrW = nil

	// Attach suspended process to Job Object before allowing any thread instructions to run.
	if err := job.assignProcess(procInfo.Process); err != nil {
		_ = windows.TerminateProcess(procInfo.Process, 1)
		return nil, err
	}

	if _, err := windows.ResumeThread(procInfo.Thread); err != nil {
		job.terminate(1)
		return nil, fmt.Errorf("sandbox: ResumeThread failed: %w", err)
	}

	// Stream stdin/stdout/stderr concurrently.
	var stdoutBuf, stderrBuf bytes.Buffer
	var ioWg sync.WaitGroup

	if spec.Stdin != nil {
		ioWg.Add(1)
		go func() {
			defer ioWg.Done()
			_, _ = io.Copy(stdinW, spec.Stdin)
			_ = stdinW.Close()
			stdinW = nil
		}()
	} else {
		_ = stdinW.Close()
		stdinW = nil
	}

	stdoutDest := io.Writer(&stdoutBuf)
	if spec.Stdout != nil {
		stdoutDest = io.MultiWriter(&stdoutBuf, spec.Stdout)
	}
	stderrDest := io.Writer(&stderrBuf)
	if spec.Stderr != nil {
		stderrDest = io.MultiWriter(&stderrBuf, spec.Stderr)
	}

	ioWg.Add(2)
	go func() {
		defer ioWg.Done()
		_, _ = io.Copy(stdoutDest, stdoutR)
	}()
	go func() {
		defer ioWg.Done()
		_, _ = io.Copy(stderrDest, stderrR)
	}()

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	waitCh := make(chan error, 1)
	go func() {
		_, wErr := windows.WaitForSingleObject(procInfo.Process, windows.INFINITE)
		waitCh <- wErr
	}()

	var timedOut bool
	select {
	case <-waitCh:
	case <-execCtx.Done():
		if spec.Timeout > 0 && execCtx.Err() == context.DeadlineExceeded {
			timedOut = true
		}
		job.terminate(137)
		<-waitCh
	}

	// Ensure all child processes in the job are terminated so stdout/stderr pipes close cleanly.
	job.terminate(1)
	ioWg.Wait()

	duration := time.Since(startTime)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(procInfo.Process, &exitCode); err != nil {
		return nil, fmt.Errorf("sandbox: GetExitCodeProcess failed: %w", err)
	}

	killedByOOM := job.killedByOOM.Load()
	if timedOut || killedByOOM {
		if exitCode == 0 {
			exitCode = 137
		}
	}

	res := &ExecResult{
		ExitCode:    int(exitCode),
		Duration:    duration,
		TimedOut:    timedOut,
		KilledByOOM: killedByOOM,
		Stdout:      stdoutBuf.String(),
		Stderr:      stderrBuf.String(),
	}

	if timedOut {
		return res, fmt.Errorf("sandbox: execution timed out after %v", spec.Timeout)
	}
	if execCtx.Err() != nil && !timedOut {
		return res, execCtx.Err()
	}

	return res, nil
}

func resolveWindowsCommandPath(cmd string, cwd string) (string, error) {
	if filepath.IsAbs(cmd) {
		if _, err := os.Stat(cmd); err == nil {
			return cmd, nil
		}
	}
	if strings.ContainsAny(cmd, `\/`) && cwd != "" {
		cand := filepath.Join(cwd, cmd)
		if path, err := exec.LookPath(cand); err == nil {
			return path, nil
		}
	}
	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return absPath, nil
}

func buildWindowsCommandLine(exePath string, args []string) (*uint16, error) {
	var b strings.Builder
	b.WriteString(syscall.EscapeArg(exePath))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(syscall.EscapeArg(a))
	}
	return windows.UTF16PtrFromString(b.String())
}

func createWindowsEnvBlock(env []string) ([]uint16, error) {
	if len(env) == 0 {
		// Provide a minimal valid empty double-NUL terminated block.
		return []uint16{0, 0}, nil
	}
	var block []uint16
	for _, kv := range env {
		if strings.ContainsRune(kv, 0) {
			return nil, fmt.Errorf("sandbox: environment variable contains NUL byte")
		}
		u16, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		block = append(block, u16...)
	}
	block = append(block, 0)
	return block, nil
}

// createInheritablePipe creates an os.File pipe pair where only the child-facing
// end is marked inheritable (HANDLE_FLAG_INHERIT).
func createInheritablePipe(inheritRead bool) (r *os.File, w *os.File, err error) {
	r, w, err = os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("sandbox: os.Pipe failed: %w", err)
	}

	childFile := w
	parentFile := r
	if inheritRead {
		childFile = r
		parentFile = w
	}

	if err := windows.SetHandleInformation(windows.Handle(parentFile.Fd()), windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, nil, fmt.Errorf("sandbox: SetHandleInformation(parent) failed: %w", err)
	}
	if err := windows.SetHandleInformation(windows.Handle(childFile.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, nil, fmt.Errorf("sandbox: SetHandleInformation(child) failed: %w", err)
	}

	return r, w, nil
}
