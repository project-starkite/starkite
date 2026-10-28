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
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
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
	return checkWindowsHostBaseline() == nil
}

// ValidateSpec verifies that the ExecutionSpec is valid and supported on Windows.
func (d *WindowsDriver) ValidateSpec(spec *ExecutionSpec) error {
	if spec == nil {
		return fmt.Errorf("sandbox: execution spec cannot be nil")
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	if err := checkWindowsHostBaseline(); err != nil {
		return err
	}
	if err := validateSpecVolumes(spec); err != nil {
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
	var closeStdinOnce sync.Once
	closeStdinW := func() {
		closeStdinOnce.Do(func() {
			if stdinW != nil {
				_ = stdinW.Close()
			}
		})
	}
	defer func() {
		if stdinR != nil {
			_ = stdinR.Close()
		}
		closeStdinW()
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

	// Sweep stale leases from prior dead processes.
	_ = sweepStaleWindowsLeases()

	var grantedPaths []string
	var scratchDir string
	profile, err := newAppContainerProfile(spec.Network)
	if err != nil {
		return nil, err
	}
	defer profile.close()
	secCaps := profile.securityCapabilitiesPtr()

	grantedSet := make(map[string]bool)
	appliedMasks := make(map[string]windows.ACCESS_MASK)
	traversedSet := make(map[string]bool)

	addGrant := func(p string, mask windows.ACCESS_MASK) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		key := strings.ToLower(clean)
		if (appliedMasks[key] & mask) == mask {
			return
		}
		merged := appliedMasks[key] | mask
		if err := grantPathAccess(clean, profile.appContainer, merged); err == nil {
			appliedMasks[key] = merged
			if !grantedSet[key] {
				grantedSet[key] = true
				grantedPaths = append(grantedPaths, clean)
			}
		}
	}

	addDirTraverse := func(p string) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		key := strings.ToLower(clean)
		if appliedMasks[key] != 0 || traversedSet[key] {
			return
		}
		if err := grantDirTraverse(clean, profile.appContainer); err == nil {
			traversedSet[key] = true
			if !grantedSet[key] {
				grantedSet[key] = true
				grantedPaths = append(grantedPaths, clean)
			}
		}
	}

	addGrant(exePath, execAccessMask)

	cleanCwd := ""
	if spec.Cwd != "" {
		if abs, err := filepath.Abs(spec.Cwd); err == nil {
			cleanCwd = filepath.Clean(abs)
		} else {
			cleanCwd = filepath.Clean(spec.Cwd)
		}
	}
	userHome := os.Getenv("USERPROFILE")
	if userHome == "" {
		userHome = os.Getenv("HOME")
	}
	cleanHome := ""
	if userHome != "" {
		if abs, err := filepath.Abs(userHome); err == nil {
			cleanHome = filepath.Clean(abs)
		} else {
			cleanHome = filepath.Clean(userHome)
		}
	}
	isHomeOrRoot := false
	if cleanCwd != "" {
		if isDriveRoot(cleanCwd) || (cleanHome != "" && strings.EqualFold(cleanCwd, cleanHome)) {
			isHomeOrRoot = true
		}
	}
	if isHomeOrRoot {
		if isDriveRoot(cleanCwd) {
			fmt.Fprintln(os.Stderr, "sandbox: working directory is /; isolated to script file only. No other files in / are accessible.")
		} else {
			fmt.Fprintln(os.Stderr, "sandbox: working directory is $HOME; isolated to script file only. No other files in $HOME are accessible.")
		}
	}

	// Ephemeral scratch directory with strict W^X enforcement in a single ACL transaction.
	candScratch := filepath.Join(os.TempDir(), "starkite-sbx-"+profile.name)
	if err := os.MkdirAll(candScratch, 0700); err == nil {
		_ = setScratchDirDacl(candScratch, profile.appContainer)
		scratchDir = candScratch
		defer os.RemoveAll(candScratch)
	}

	// Unveil explicit mounts first so directory mounts take precedence over non-inheriting traverse rules.
	for _, m := range spec.Mounts {
		if m.Source == "" {
			continue
		}
		cleanMount := filepath.Clean(m.Source)
		if isHomeOrRoot && ((cleanHome != "" && strings.EqualFold(cleanMount, cleanHome)) || isDriveRoot(cleanMount)) {
			continue
		}
		var mask windows.ACCESS_MASK = readOnlyAccessMask
		if m.Mode == MountRW {
			mask = readWriteAccessMask
		}
		addGrant(cleanMount, mask)
	}

	// Unveil script file and runtime manifests (read-only), with non-inheriting directory traverse on parent dir.
	scriptFile := spec.ScriptFile
	if scriptFile != "" && !filepath.IsAbs(scriptFile) && spec.Cwd != "" {
		scriptFile = filepath.Join(spec.Cwd, scriptFile)
	}
	if scriptFile != "" {
		addGrant(scriptFile, readOnlyAccessMask)
		if !isHomeOrRoot {
			scriptDir := filepath.Dir(scriptFile)
			addDirTraverse(scriptDir)
			for _, asset := range []string{"mod.yaml", "mod.lock", "star.mod", "config.yaml"} {
				assetPath := filepath.Join(scriptDir, asset)
				if _, statErr := os.Stat(assetPath); statErr == nil {
					addGrant(assetPath, readOnlyAccessMask)
				}
			}
		}
	}
	if !isHomeOrRoot && cleanCwd != "" {
		addDirTraverse(cleanCwd)
		cfgPath := filepath.Join(cleanCwd, "config.yaml")
		if _, statErr := os.Stat(cfgPath); statErr == nil {
			addGrant(cfgPath, readOnlyAccessMask)
		}
	}

	// Unveil ~/.starkite runtime assets (strictly read-only on config.yaml and modules cache).
	if !isHomeOrRoot && cleanHome != "" {
		starkiteDir := filepath.Join(cleanHome, ".starkite")
		modulesDir := filepath.Join(starkiteDir, "modules")
		_ = os.MkdirAll(modulesDir, 0755)
		addDirTraverse(starkiteDir)
		addGrant(modulesDir, readOnlyAccessMask)
		userCfg := filepath.Join(starkiteDir, "config.yaml")
		if _, statErr := os.Stat(userCfg); statErr == nil {
			addGrant(userCfg, readOnlyAccessMask)
		}
	}

	// Unveil allowed executables.
	if len(spec.AllowedExecPaths) > 0 {
		resolvedExecs := ResolveExecutablePaths(spec.AllowedExecPaths, spec.Cwd, userHome)
		for _, exe := range resolvedExecs {
			addGrant(exe, execAccessMask)
		}
	}

	// Record lease in journal.
	lease := windowsLease{
		PID:             os.Getpid(),
		ProfileName:     profile.name,
		AppContainerSID: profile.appContainer.String(),
		Paths:           grantedPaths,
		CreatedAt:       time.Now(),
	}
	_ = recordWindowsLease(lease)

	// Deferred DACL revocation and lease removal.
	defer func() {
		for _, p := range grantedPaths {
			_ = revokePathAccess(p, profile.appContainer)
		}
		_ = removeWindowsLease(profile.name)
	}()

	attrs, err := buildWindowsStartupAttrs(spec, stdinHandle, stdoutHandle, stderrHandle, secCaps)
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
	cwd := spec.Cwd
	cwdAccessible := false
	if cleanCwd != "" {
		for _, p := range grantedPaths {
			cleanP := filepath.Clean(p)
			if equalOrHasPathPrefixFold(cleanCwd, cleanP) {
				cwdAccessible = true
				break
			}
		}
	}
	if !cwdAccessible {
		cwd = os.Getenv("SystemRoot")
		if cwd == "" {
			cwd = `C:\Windows`
		}
	} else {
		cwd = cleanCwd
	}
	cwdPtr, err = windows.UTF16PtrFromString(cwd)
	if err != nil {
		return nil, err
	}

	envVars := spec.Env
	if len(envVars) == 0 {
		baseMap := SanitizedBaselineEnv(os.Environ())
		envVars = make([]string, 0, len(baseMap))
		for k, v := range baseMap {
			envVars = append(envVars, k+"="+v)
		}
	}
	if scratchDir != "" {
		envVars = append([]string{"TEMP=" + scratchDir, "TMP=" + scratchDir}, envVars...)
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
		return nil, FormatPermissionError(fmt.Errorf("sandbox: CreateProcessW (%s) failed: %w", exePath, err), exePath, spec)
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
		go func(w *os.File, r io.Reader) {
			_, _ = io.Copy(w, r)
			closeStdinW()
		}(stdinW, spec.Stdin)
	} else {
		closeStdinW()
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
	closeStdinW()
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

	if res.ExitCode != 0 && !timedOut && !killedByOOM {
		prevStderr := res.Stderr
		res.Stderr = enrichPermissionStderr(res.Stderr, spec)
		if spec.Stderr != nil && len(res.Stderr) > len(prevStderr) {
			_, _ = io.WriteString(spec.Stderr, res.Stderr[len(prevStderr):])
		}
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
	seen := make(map[string]bool)
	var deduped []string
	for _, kv := range env {
		if strings.ContainsRune(kv, 0) {
			return nil, fmt.Errorf("sandbox: environment variable contains NUL byte")
		}
		k, _, hasEq := strings.Cut(kv, "=")
		if !hasEq {
			continue
		}
		upper := strings.ToUpper(k)
		if !seen[upper] {
			seen[upper] = true
			deduped = append(deduped, kv)
		}
	}
	sort.Slice(deduped, func(i, j int) bool {
		return strings.ToUpper(deduped[i]) < strings.ToUpper(deduped[j])
	})
	var block []uint16
	for _, kv := range deduped {
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

func isDriveRoot(p string) bool {
	clean := filepath.Clean(p)
	if clean == "/" || clean == `\` {
		return true
	}
	vol := filepath.VolumeName(clean)
	if vol != "" {
		rest := clean[len(vol):]
		if rest == `\` || rest == "/" || rest == "" || rest == "." {
			return true
		}
	}
	return false
}
