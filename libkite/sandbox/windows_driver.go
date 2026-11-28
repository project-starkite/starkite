//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func resolveCanonicalWindowsPath(p string) string {
	if p == "" {
		return ""
	}
	clean := filepath.Clean(p)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = filepath.Clean(abs)
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil && resolved != "" {
		return filepath.Clean(resolved)
	}
	return clean
}

func isCurrentExecutablePath(exePath string) bool {
	self, err := os.Executable()
	if err != nil || self == "" || exePath == "" {
		return false
	}
	fiSelf, err1 := os.Stat(self)
	fiTarget, err2 := os.Stat(exePath)
	if err1 == nil && err2 == nil {
		return os.SameFile(fiSelf, fiTarget)
	}
	return strings.EqualFold(resolveCanonicalWindowsPath(self), resolveCanonicalWindowsPath(exePath))
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
	exePath = resolveCanonicalWindowsPath(exePath)

	isSelfHandoff := spec.SelfHandoff || (isCurrentExecutablePath(exePath) && os.Getenv(InsideEnvVar) == "1")
	if !isSelfHandoff {
		policySpec := *spec
		policySpec.Command = append([]string{exePath}, spec.Command[1:]...)
		if policyErr := checkContainerExecPolicy(&policySpec); policyErr != nil {
			return policyDeniedResult(spec, policyErr), nil
		}
	}

	job, err := newWindowsJob(spec)
	if err != nil {
		return nil, err
	}
	defer job.close()

	// Set up standard I/O pipes non-inheritably; child ends are marked inheritable
	// only inside the syscall.ForkLock window around CreateProcess.
	stdinR, stdinW, err := createNonInheritablePipe()
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

	stdoutR, stdoutW, err := createNonInheritablePipe()
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

	stderrR, stderrW, err := createNonInheritablePipe()
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

	recordGranted := func(clean string) {
		key := strings.ToLower(clean)
		if !grantedSet[key] {
			grantedSet[key] = true
			grantedPaths = append(grantedPaths, clean)
		}
	}

	// Deferred DACL revocation and lease removal must stay active before any grants occur.
	defer func() {
		for _, p := range grantedPaths {
			_ = revokePathAccess(p, profile.appContainer)
		}
		_ = removeWindowsLease(profile.name)
	}()

	addGrant := func(p string, mask windows.ACCESS_MASK) error {
		if p == "" {
			return nil
		}
		clean := resolveCanonicalWindowsPath(p)
		key := strings.ToLower(clean)
		if (appliedMasks[key] & mask) == mask {
			return nil
		}
		merged := appliedMasks[key] | mask
		if err := grantPathAccess(clean, profile.appContainer, merged); err != nil {
			return fmt.Errorf("sandbox: failed to grant ACL on %q: %w", clean, err)
		}
		appliedMasks[key] = merged
		recordGranted(clean)
		return nil
	}

	addMountRWDacl := func(p string) error {
		if p == "" {
			return nil
		}
		clean := resolveCanonicalWindowsPath(p)
		key := strings.ToLower(clean)
		if fi, statErr := os.Stat(clean); statErr != nil {
			if os.IsNotExist(statErr) {
				return nil
			}
			return statErr
		} else if fi.IsDir() {
			if err := setScratchDirDacl(clean, profile.appContainer); err != nil {
				return fmt.Errorf("sandbox: failed to enforce W^X DACL on %q: %w", clean, err)
			}
			appliedMasks[key] |= readWriteAccessMask
			recordGranted(clean)
			return nil
		}
		if err := addGrant(clean, readWriteAccessMask); err != nil {
			return err
		}
		if err := denyPathAccess(clean, profile.appContainer, denyExecAccessMask); err != nil {
			return fmt.Errorf("sandbox: failed to deny execute ACL on %q: %w", clean, err)
		}
		return nil
	}

	addDirTraverse := func(p string) error {
		if p == "" {
			return nil
		}
		clean := resolveCanonicalWindowsPath(p)
		key := strings.ToLower(clean)
		if appliedMasks[key] != 0 || traversedSet[key] {
			return nil
		}
		if err := grantDirTraverse(clean, profile.appContainer); err != nil {
			return fmt.Errorf("sandbox: failed to grant traverse ACL on %q: %w", clean, err)
		}
		traversedSet[key] = true
		recordGranted(clean)
		return nil
	}

	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	canonSysRoot := resolveCanonicalWindowsPath(sysRoot)

	cleanCwd := ""
	if spec.Cwd != "" {
		cleanCwd = resolveCanonicalWindowsPath(spec.Cwd)
	}
	userHome := os.Getenv("USERPROFILE")
	if userHome == "" {
		userHome = os.Getenv("HOME")
	}
	cleanHome := ""
	if userHome != "" {
		cleanHome = resolveCanonicalWindowsPath(userHome)
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

	// Prepare ephemeral scratch directory before applying any DACLs so its path can be recorded in the lease first.
	candScratch := filepath.Join(os.TempDir(), "starkite-sbx-"+profile.name)
	if err := os.MkdirAll(candScratch, 0700); err != nil {
		return nil, fmt.Errorf("sandbox: failed to create scratch directory %q: %w", candScratch, err)
	}
	defer os.RemoveAll(candScratch)
	candScratch = resolveCanonicalWindowsPath(candScratch)
	scratchDir = candScratch

	// Precompute every canonical path whose DACL will be touched and persist the lease before mutating any ACLs.
	plannedSet := make(map[string]bool)
	var plannedPaths []string
	planPath := func(p string) {
		if p == "" {
			return
		}
		clean := resolveCanonicalWindowsPath(p)
		key := strings.ToLower(clean)
		if !plannedSet[key] {
			plannedSet[key] = true
			plannedPaths = append(plannedPaths, clean)
		}
	}

	if isHomeOrRoot {
		planPath(cleanCwd)
	}
	planPath(candScratch)

	for _, m := range spec.Mounts {
		target := m.Source
		if target == "" && m.Type == MountTmpfs {
			target = m.Destination
		}
		if target == "" {
			continue
		}
		cleanMount := resolveCanonicalWindowsPath(target)
		if isHomeOrRoot && ((cleanHome != "" && strings.EqualFold(cleanMount, cleanHome)) || isDriveRoot(cleanMount)) {
			continue
		}
		if m.Mode == MountRW || m.Type == MountTmpfs || !equalOrHasPathPrefixFold(cleanMount, canonSysRoot) {
			planPath(cleanMount)
		}
	}

	scriptFile := spec.ScriptFile
	if scriptFile != "" && !filepath.IsAbs(scriptFile) && spec.Cwd != "" {
		scriptFile = filepath.Join(spec.Cwd, scriptFile)
	}
	if scriptFile != "" && !equalOrHasPathPrefixFold(resolveCanonicalWindowsPath(scriptFile), canonSysRoot) {
		planPath(scriptFile)
		if !isHomeOrRoot {
			scriptDir := filepath.Dir(scriptFile)
			planPath(scriptDir)
			for _, asset := range []string{"mod.yaml", "mod.lock", "star.mod", "config.yaml"} {
				assetPath := filepath.Join(scriptDir, asset)
				if _, statErr := os.Stat(assetPath); statErr == nil {
					planPath(assetPath)
				}
			}
		}
	}
	if !isHomeOrRoot && cleanCwd != "" {
		planPath(cleanCwd)
		cfgPath := filepath.Join(cleanCwd, "config.yaml")
		if _, statErr := os.Stat(cfgPath); statErr == nil {
			planPath(cfgPath)
		}
	}

	var starkiteDir, modulesDir, userCfg string
	if !isHomeOrRoot && cleanHome != "" {
		starkiteDir = filepath.Join(cleanHome, ".starkite")
		modulesDir = filepath.Join(starkiteDir, "modules")
		_ = os.MkdirAll(modulesDir, 0755)
		planPath(starkiteDir)
		planPath(modulesDir)
		userCfg = filepath.Join(starkiteDir, "config.yaml")
		if _, statErr := os.Stat(userCfg); statErr == nil {
			planPath(userCfg)
		}
	}

	var resolvedExecs []string
	if len(spec.AllowedExecPaths) > 0 {
		resolvedExecs = ResolveExecutablePaths(spec.AllowedExecPaths, spec.Cwd, userHome)
		for _, exe := range resolvedExecs {
			cleanExe := resolveCanonicalWindowsPath(exe)
			if equalOrHasPathPrefixFold(cleanExe, canonSysRoot) {
				continue
			}
			planPath(cleanExe)
			planPath(filepath.Dir(cleanExe))
		}
	}
	if exePath != "" && !equalOrHasPathPrefixFold(exePath, canonSysRoot) {
		planPath(exePath)
		planPath(filepath.Dir(exePath))
	}

	creationTime, _ := getProcessCreationTime(os.Getpid())
	lease := windowsLease{
		PID:                 os.Getpid(),
		ProcessCreationTime: creationTime,
		ProfileName:         profile.name,
		AppContainerSID:     profile.appContainer.String(),
		ScratchDir:          candScratch,
		Paths:               plannedPaths,
		CreatedAt:           time.Now().UTC(),
	}
	if err := recordWindowsLease(lease); err != nil {
		return nil, fmt.Errorf("sandbox: record lease journal: %w", err)
	}

	if isHomeOrRoot {
		if err := addDirTraverse(cleanCwd); err != nil {
			return nil, err
		}
	}

	if err := setScratchDirDacl(candScratch, profile.appContainer); err != nil {
		return nil, fmt.Errorf("sandbox: failed to configure scratch DACL on %q: %w", candScratch, err)
	}
	recordGranted(candScratch)

	// Unveil explicit mounts first so directory mounts take precedence over non-inheriting traverse rules.
	for _, m := range spec.Mounts {
		target := m.Source
		if target == "" && m.Type == MountTmpfs {
			target = m.Destination
		}
		if target == "" {
			continue
		}
		cleanMount := resolveCanonicalWindowsPath(target)
		if isHomeOrRoot && ((cleanHome != "" && strings.EqualFold(cleanMount, cleanHome)) || isDriveRoot(cleanMount)) {
			continue
		}
		if m.Mode == MountRW || m.Type == MountTmpfs {
			if err := addMountRWDacl(cleanMount); err != nil {
				return nil, err
			}
		} else if !equalOrHasPathPrefixFold(cleanMount, canonSysRoot) {
			if err := addGrant(cleanMount, readOnlyAccessMask); err != nil {
				return nil, err
			}
		}
	}

	// Unveil script file and runtime manifests (read-only), with non-inheriting directory traverse on parent dir.
	if scriptFile != "" && !equalOrHasPathPrefixFold(resolveCanonicalWindowsPath(scriptFile), canonSysRoot) {
		if err := addGrant(scriptFile, readOnlyAccessMask); err != nil {
			return nil, err
		}
		if !isHomeOrRoot {
			scriptDir := filepath.Dir(scriptFile)
			if err := addDirTraverse(scriptDir); err != nil {
				return nil, err
			}
			for _, asset := range []string{"mod.yaml", "mod.lock", "star.mod", "config.yaml"} {
				assetPath := filepath.Join(scriptDir, asset)
				if _, statErr := os.Stat(assetPath); statErr == nil {
					if err := addGrant(assetPath, readOnlyAccessMask); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if !isHomeOrRoot && cleanCwd != "" {
		if err := addDirTraverse(cleanCwd); err != nil {
			return nil, err
		}
		cfgPath := filepath.Join(cleanCwd, "config.yaml")
		if _, statErr := os.Stat(cfgPath); statErr == nil {
			if err := addGrant(cfgPath, readOnlyAccessMask); err != nil {
				return nil, err
			}
		}
	}

	// Unveil ~/.starkite runtime assets (strictly read-only on config.yaml and modules cache).
	if !isHomeOrRoot && cleanHome != "" {
		_ = addDirTraverse(starkiteDir)
		_ = addGrant(modulesDir, readOnlyAccessMask)
		if _, statErr := os.Stat(userCfg); statErr == nil {
			_ = addGrant(userCfg, readOnlyAccessMask)
		}
	}

	// Unveil allowed executables outside SystemRoot; Windows system binaries are
	// TrustedInstaller-owned and already grant ALL APPLICATION PACKAGES execute.
	for _, exe := range resolvedExecs {
		cleanExe := resolveCanonicalWindowsPath(exe)
		if equalOrHasPathPrefixFold(cleanExe, canonSysRoot) {
			continue
		}
		if err := addGrant(cleanExe, execAccessMask); err != nil {
			return nil, err
		}
		if err := addDirTraverse(filepath.Dir(cleanExe)); err != nil {
			return nil, err
		}
	}

	// Unveil target executable if located outside system directories, without granting execute on its parent directory.
	if exePath != "" && !equalOrHasPathPrefixFold(exePath, canonSysRoot) {
		if err := addGrant(exePath, execAccessMask); err != nil {
			return nil, err
		}
		if err := addDirTraverse(filepath.Dir(exePath)); err != nil {
			return nil, err
		}
	}

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
	var cwd string
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

	envVars := append([]string(nil), spec.Env...)
	if len(envVars) == 0 {
		envVars = mapToEnvSlice(SanitizedBaselineEnv(os.Environ()))
	}
	if scratchDir != "" {
		envVars = append(envVars, "TEMP="+scratchDir, "TMP="+scratchDir)
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
	err = createSuspendedWindowsProcess(
		appNamePtr,
		cmdLine,
		creationFlags,
		envPtr,
		cwdPtr,
		&attrs.startupInfoEx.StartupInfo,
		&procInfo,
		[]*os.File{stdinR, stdoutW, stderrW},
	)
	if err != nil {
		return nil, FormatPermissionError(fmt.Errorf("sandbox: CreateProcessW (%s) failed: %w", exePath, err), exePath, spec)
	}
	// Parent ends are closed inside createSuspendedWindowsProcess under ForkLock.
	stdinR = nil
	stdoutW = nil
	stderrW = nil
	defer func() {
		_ = windows.CloseHandle(procInfo.Thread)
		_ = windows.CloseHandle(procInfo.Process)
	}()

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
	var stdinDone chan struct{}

	if spec.Stdin != nil {
		stdinDone = make(chan struct{})
		go func(w *os.File, r io.Reader) {
			defer close(stdinDone)
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
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			timedOut = true
		}
		job.terminate(137)
		<-waitCh
	}

	// Ensure all child processes in the job are terminated so stdout/stderr pipes close cleanly.
	job.terminate(1)
	closeStdinW()
	if stdinDone != nil && spec.Stdin != os.Stdin {
		<-stdinDone
	}
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

	if !timedOut && !killedByOOM {
		applyPermissionStderr(res, spec)
	}

	if timedOut {
		if spec.Timeout > 0 {
			return res, fmt.Errorf("sandbox: execution timed out after %v: %w", spec.Timeout, execCtx.Err())
		}
		return res, fmt.Errorf("sandbox: execution timed out: %w", execCtx.Err())
	}
	if execCtx.Err() != nil {
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
	seenIdx := make(map[string]int, len(env))
	type envEntry struct {
		upperKey string
		kv       string
	}
	var deduped []envEntry
	for _, kv := range env {
		if strings.ContainsRune(kv, 0) {
			return nil, fmt.Errorf("sandbox: environment variable contains NUL byte")
		}
		k, _, hasEq := strings.Cut(kv, "=")
		if !hasEq {
			continue
		}
		upper := strings.ToUpper(k)
		if idx, exists := seenIdx[upper]; exists {
			deduped[idx].kv = kv
		} else {
			seenIdx[upper] = len(deduped)
			deduped = append(deduped, envEntry{upperKey: upper, kv: kv})
		}
	}
	slices.SortFunc(deduped, func(a, b envEntry) int {
		return strings.Compare(a.upperKey, b.upperKey)
	})
	var block []uint16
	for _, entry := range deduped {
		u16, err := windows.UTF16FromString(entry.kv)
		if err != nil {
			return nil, err
		}
		block = append(block, u16...)
	}
	block = append(block, 0)
	return block, nil
}

// createSuspendedWindowsProcess acquires syscall.ForkLock around toggling
// HANDLE_FLAG_INHERIT and CreateProcess so concurrent os/exec calls cannot
// inherit the sandbox child pipe ends.
func createSuspendedWindowsProcess(
	appNamePtr, cmdLine *uint16,
	creationFlags uint32,
	envPtr, cwdPtr *uint16,
	startupInfo *windows.StartupInfo,
	procInfo *windows.ProcessInformation,
	childFiles []*os.File,
) error {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()

	for i, f := range childFiles {
		if f == nil {
			continue
		}
		if err := windows.SetHandleInformation(windows.Handle(f.Fd()), windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			for j := range i {
				if childFiles[j] != nil {
					_ = windows.SetHandleInformation(windows.Handle(childFiles[j].Fd()), windows.HANDLE_FLAG_INHERIT, 0)
				}
			}
			return fmt.Errorf("sandbox: SetHandleInformation(child) failed: %w", err)
		}
	}

	err := windows.CreateProcess(
		appNamePtr,
		cmdLine,
		nil,
		nil,
		true, // bInheritHandles (restricted via PROC_THREAD_ATTRIBUTE_HANDLE_LIST)
		creationFlags,
		envPtr,
		cwdPtr,
		startupInfo,
		procInfo,
	)
	if err != nil {
		for _, f := range childFiles {
			if f != nil {
				_ = windows.SetHandleInformation(windows.Handle(f.Fd()), windows.HANDLE_FLAG_INHERIT, 0)
			}
		}
		return err
	}

	for _, f := range childFiles {
		if f != nil {
			_ = f.Close()
		}
	}
	return nil
}

// createNonInheritablePipe creates an os.File pipe pair with HANDLE_FLAG_INHERIT cleared on both ends.
func createNonInheritablePipe() (r *os.File, w *os.File, err error) {
	r, w, err = os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("sandbox: os.Pipe failed: %w", err)
	}
	for _, f := range []*os.File{r, w} {
		if err := windows.SetHandleInformation(windows.Handle(f.Fd()), windows.HANDLE_FLAG_INHERIT, 0); err != nil {
			_ = r.Close()
			_ = w.Close()
			return nil, nil, fmt.Errorf("sandbox: SetHandleInformation failed: %w", err)
		}
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
