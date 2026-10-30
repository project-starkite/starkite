//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const landlockExecEnvVar = "STARKITE_LANDLOCK_EXEC_SPEC"

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

type landlockExecPayload struct {
	TargetBin        string        `json:"target_bin"`
	TargetArgv       []string      `json:"target_argv"`
	Cwd              string        `json:"cwd"`
	Network          NetworkMode   `json:"network"`
	NetworkAllow     []string      `json:"network_allow,omitempty"`
	NetworkDeny      []string      `json:"network_deny,omitempty"`
	Mounts           []Mount       `json:"mounts"`
	MaxMemoryMB      int64         `json:"max_memory_mb"`
	MaxPIDs          int64         `json:"max_pids"`
	Timeout          time.Duration `json:"timeout"`
	ScriptFile       string        `json:"script_file"`
	NoExec           bool          `json:"no_exec"`
	AllowedExecPaths []string      `json:"allowed_exec_paths"`
	AuditLog         bool          `json:"audit_log"`
}

// LandlockDriver implements the sandbox.Driver interface on Linux using
// the kernel Landlock Linux Security Module (LSM).
type LandlockDriver struct{}

func init() {
	if raw := os.Getenv(landlockExecEnvVar); raw != "" {
		runLandlockSelfReexec(raw)
	}
	Register(NewLandlockDriver())
}

func runLandlockSelfReexec(raw string) {
	_ = os.Unsetenv(landlockExecEnvVar)
	_ = os.Setenv(InsideEnvVar, "1")

	var payload landlockExecPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: invalid landlock exec payload: %v\n", err)
		os.Exit(125)
	}

	spec := &ExecutionSpec{
		Command:          payload.TargetArgv,
		Cwd:              payload.Cwd,
		Network:          payload.Network,
		NetworkAllow:     payload.NetworkAllow,
		NetworkDeny:      payload.NetworkDeny,
		Mounts:           payload.Mounts,
		MaxMemoryMB:      payload.MaxMemoryMB,
		MaxPIDs:          payload.MaxPIDs,
		Timeout:          payload.Timeout,
		ScriptFile:       payload.ScriptFile,
		NoExec:           payload.NoExec,
		AllowedExecPaths: payload.AllowedExecPaths,
		AuditLog:         payload.AuditLog,
	}

	d := NewLandlockDriver()
	if err := d.ApplyInProcess(spec); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(125)
	}

	err := unix.Exec(payload.TargetBin, payload.TargetArgv, os.Environ())
	fmt.Fprintf(os.Stderr, "%v\n", FormatPermissionError(err, payload.TargetBin, spec))
	os.Exit(126)
}

// NewLandlockDriver creates a new LandlockDriver.
func NewLandlockDriver() *LandlockDriver {
	return &LandlockDriver{}
}

// Name returns the driver identifier "landlock".
func (d *LandlockDriver) Name() string {
	return DriverLandlock
}

// landlockABI queries the highest Landlock ABI version supported by the running Linux kernel.
func landlockABI() int {
	v, _, err := unix.Syscall(
		sysLandlockCreateRuleset,
		0, 0,
		landlockCreateRulesetVersion,
	)
	if err != 0 {
		return 0
	}
	return int(v)
}

// Available reports whether the host Linux kernel supports Landlock.
func (d *LandlockDriver) Available() bool {
	return landlockABI() >= 1
}

// ValidateSpec verifies that the ExecutionSpec is valid for Landlock isolation.
func (d *LandlockDriver) ValidateSpec(spec *ExecutionSpec) error {
	return spec.Validate()
}

// isSharedMemoryPath reports whether p targets host shared memory (/dev/shm or /run/shm).
func isSharedMemoryPath(p string) bool {
	clean := filepath.Clean(p)
	return clean == "/dev/shm" || strings.HasPrefix(clean, "/dev/shm/") ||
		clean == "/run/shm" || strings.HasPrefix(clean, "/run/shm/")
}

// ApplyInProcess restricts the current process and all future children
// using Landlock system calls and Seccomp-BPF. Once applied, restrictions are irreversible.
func (d *LandlockDriver) ApplyInProcess(spec *ExecutionSpec) error {
	abi := landlockABI()
	if abi < 1 {
		return errors.New("sandbox: landlock driver not supported by kernel")
	}

	// Separate read, execute, and write bitmasks to enforce Write-XOR-Execute (W^X):
	// writable mounts receive writeDataAccess (which omits landlockAccessFsExecute).
	readDataAccess := uint64(
		landlockAccessFsReadFile |
			landlockAccessFsReadDir,
	)
	execAccess := readDataAccess | uint64(landlockAccessFsExecute)
	writeDataAccess := readDataAccess | uint64(
		landlockAccessFsWriteFile|
			landlockAccessFsRemoveDir|
			landlockAccessFsRemoveFile|
			landlockAccessFsMakeDir|
			landlockAccessFsMakeReg|
			landlockAccessFsMakeSym,
	)
	if abi >= 3 {
		writeDataAccess |= landlockAccessFsTruncate
	}

	handledAccess := writeDataAccess | uint64(landlockAccessFsExecute)
	if abi >= 5 {
		handledAccess |= landlockAccessFsIoctlDev
	}

	attr := landlockRulesetAttr{
		handledAccessFs: handledAccess,
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

	// System executable paths and shared library directories
	systemExecPaths := []string{
		"/usr",
		"/lib",
		"/lib64",
		"/bin",
		"/sbin",
	}
	systemLibPaths := []string{
		"/lib",
		"/lib64",
		"/usr/lib",
		"/usr/lib64",
		"/usr/libexec",
	}
	// Essential device paths and runtime assets
	systemDataPaths := []string{
		"/dev/urandom",
		"/dev/random",
	}
	systemNetworkPaths := []string{
		"/etc/ssl",
		"/etc/pki",
		"/etc/ca-certificates",
		"/usr/share/ca-certificates",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/services",
	}
	systemWritableDevices := []string{
		"/dev/null",
		"/dev/zero",
	}

	switch {
	case spec.NoExec:
		for _, p := range systemExecPaths {
			if _, statErr := os.Stat(p); statErr == nil {
				_ = addLandlockPathRule(int(rulesetFd), p, readDataAccess)
			}
		}
	case len(spec.AllowedExecPaths) > 0:
		for _, p := range systemExecPaths {
			if _, statErr := os.Stat(p); statErr == nil {
				_ = addLandlockPathRule(int(rulesetFd), p, readDataAccess)
			}
		}
		for _, p := range systemLibPaths {
			if _, statErr := os.Stat(p); statErr == nil {
				_ = addLandlockPathRule(int(rulesetFd), p, execAccess)
			}
		}
		for _, allowed := range spec.AllowedExecPaths {
			cleanAllowed := filepath.Clean(allowed)
			if !isScratchPath(cleanAllowed) {
				if _, statErr := os.Stat(cleanAllowed); statErr == nil {
					_ = addLandlockPathRule(int(rulesetFd), cleanAllowed, execAccess|readDataAccess)
				}
				if resolved, evalErr := filepath.EvalSymlinks(cleanAllowed); evalErr == nil && resolved != cleanAllowed && !isScratchPath(resolved) {
					if _, statErr := os.Stat(resolved); statErr == nil {
						_ = addLandlockPathRule(int(rulesetFd), resolved, execAccess|readDataAccess)
					}
				}
			}
		}
	default:
		for _, p := range systemExecPaths {
			if _, statErr := os.Stat(p); statErr == nil {
				_ = addLandlockPathRule(int(rulesetFd), p, execAccess)
			}
		}
		if len(spec.Command) > 0 {
			targetCmd := filepath.Clean(spec.Command[0])
			if !isScratchPath(targetCmd) {
				if fi, statErr := os.Stat(targetCmd); statErr == nil && !fi.IsDir() {
					_ = addLandlockPathRule(int(rulesetFd), targetCmd, execAccess|readDataAccess)
				}
				if resolved, err := filepath.EvalSymlinks(targetCmd); err == nil && resolved != targetCmd && !isScratchPath(resolved) {
					if fiResolved, statErr := os.Stat(resolved); statErr == nil && !fiResolved.IsDir() {
						_ = addLandlockPathRule(int(rulesetFd), resolved, execAccess|readDataAccess)
					}
				}
			}
		}
	}

	for _, p := range systemDataPaths {
		if _, statErr := os.Stat(p); statErr == nil {
			_ = addLandlockPathRule(int(rulesetFd), p, readDataAccess)
		}
	}
	if spec.Network == NetworkHost || len(spec.NetworkAllow) > 0 {
		for _, p := range systemNetworkPaths {
			if _, statErr := os.Stat(p); statErr == nil {
				_ = addLandlockPathRule(int(rulesetFd), p, readDataAccess)
				if resolved, err := filepath.EvalSymlinks(p); err == nil && resolved != p {
					_ = addLandlockPathRule(int(rulesetFd), resolved, readDataAccess)
				}
			}
		}
	}
	for _, p := range systemWritableDevices {
		if _, statErr := os.Stat(p); statErr == nil {
			_ = addLandlockPathRule(int(rulesetFd), p, writeDataAccess)
		}
	}

	var isHomeOrRoot bool
	cleanCwd := ""
	cleanHome := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cleanHome = filepath.Clean(home)
	}
	if spec.Cwd != "" {
		cleanCwd = filepath.Clean(spec.Cwd)
		if cleanCwd == "/" || (cleanHome != "" && cleanCwd == cleanHome) {
			isHomeOrRoot = true
		}
	}

	// Runtime asset baseline: inject read-only Landlock rules for ~/.starkite, script files, and mod.lock
	// Under container parity, if CWD == $HOME or /, the script sees NO other files in $HOME (~/.starkite is omitted).
	if !isHomeOrRoot && cleanHome != "" {
		starkiteDir := filepath.Join(cleanHome, ".starkite")
		if _, statErr := os.Stat(starkiteDir); statErr == nil {
			_ = addLandlockPathRule(int(rulesetFd), starkiteDir, readDataAccess)
		}
	}
	if spec.ScriptFile != "" {
		if _, statErr := os.Stat(spec.ScriptFile); statErr == nil {
			_ = addLandlockPathRule(int(rulesetFd), spec.ScriptFile, readDataAccess)
		}
		scriptDir := filepath.Dir(spec.ScriptFile)
		for _, asset := range []string{"mod.lock", "star.mod"} {
			assetPath := filepath.Join(scriptDir, asset)
			if _, statErr := os.Stat(assetPath); statErr == nil {
				_ = addLandlockPathRule(int(rulesetFd), assetPath, readDataAccess)
			}
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
		cleanTarget := filepath.Clean(target)
		if isHomeOrRoot && (cleanTarget == cleanHome || cleanTarget == "/") {
			// Under container parity, omit host $HOME or / bind mount
			continue
		}

		// Isolate shared memory (/dev/shm and /run/shm): never expose host shared memory
		if isSharedMemoryPath(cleanTarget) || isSharedMemoryPath(m.Destination) {
			if m.Type == MountTmpfs {
				if shmScratch, shmErr := os.MkdirTemp("", "starkite-shm-*"); shmErr == nil {
					_ = addLandlockPathRule(int(rulesetFd), shmScratch, writeDataAccess)
				}
			}
			continue
		}

		if _, statErr := os.Stat(cleanTarget); statErr != nil {
			continue
		}

		if m.Mode == MountRW || m.Type == MountTmpfs {
			// W^X: writable mounts receive writeDataAccess without landlockAccessFsExecute
			if err := addLandlockPathRule(int(rulesetFd), cleanTarget, writeDataAccess); err != nil {
				return fmt.Errorf("sandbox: failed to add rw rule for %q: %w", cleanTarget, err)
			}
			if resolved, err := filepath.EvalSymlinks(cleanTarget); err == nil && resolved != cleanTarget {
				_ = addLandlockPathRule(int(rulesetFd), resolved, writeDataAccess)
			}
		} else {
			roAccess := execAccess
			if spec.NoExec || len(spec.AllowedExecPaths) > 0 {
				roAccess = readDataAccess
			}
			if err := addLandlockPathRule(int(rulesetFd), cleanTarget, roAccess); err != nil {
				return fmt.Errorf("sandbox: failed to add ro rule for %q: %w", cleanTarget, err)
			}
			if resolved, err := filepath.EvalSymlinks(cleanTarget); err == nil && resolved != cleanTarget {
				_ = addLandlockPathRule(int(rulesetFd), resolved, roAccess)
			}
		}
	}

	if isHomeOrRoot {
		if cleanCwd == "/" {
			fmt.Fprintln(os.Stderr, "sandbox: working directory is /; isolated to script file only. No other files in / are accessible.")
		} else {
			fmt.Fprintln(os.Stderr, "sandbox: working directory is $HOME; isolated to script file only. No other files in $HOME are accessible.")
		}
	}

	// Lock current goroutine to the current OS thread so all execution stays
	// confined to the Landlock-restricted thread and its child processes.
	runtime.LockOSThread()

	// Apply POSIX resource limits (anti-fork-bomb barrier)
	if err := applyResourceLimits(spec); err != nil {
		return fmt.Errorf("sandbox: failed to apply resource limits: %w", err)
	}

	// Enforce no new privileges
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("sandbox: prctl(PR_SET_NO_NEW_PRIVS) failed: %w", err)
	}

	// Always-On Seccomp-BPF: enforce process/IPC/signal boundary across all network modes,
	// plus optional network socket filtering and NoExec subprocess lockdown.
	seccompOpts := SeccompFilterOptions{
		BlockNetwork: spec.Network == NetworkNone || spec.Network == NetworkLoopback,
		NoExec:       spec.NoExec,
		AuditLog:     spec.AuditLog,
	}
	filter, filterErr := BuildSeccompFilter(seccompOpts)
	if filterErr != nil {
		return fmt.Errorf("sandbox: failed to build seccomp filter: %w", filterErr)
	}
	if seccompErr := ApplySeccompFilter(filter); seccompErr != nil {
		return fmt.Errorf("sandbox: failed to apply seccomp filter: %w", seccompErr)
	}

	// Restrict calling thread and future child processes
	if _, _, err := unix.Syscall(sysLandlockRestrictSelf, rulesetFd, 0, 0); err != 0 {
		return fmt.Errorf("sandbox: landlock_restrict_self failed: %w", err)
	}

	return nil
}

// applyResourceLimits applies unprivileged POSIX setrlimit constraints.
func applyResourceLimits(spec *ExecutionSpec) error {
	// 1. Anti-Fork-Bomb: Set a ceiling on max threads/processes (RLIMIT_NPROC).
	// On Linux, RLIMIT_NPROC is enforced per real UID across the entire host OS
	// rather than per process tree. Enforce a minimum floor of 8192 so existing
	// host user session threads do not cause immediate EAGAIN on fork/clone.
	const minHostUIDProcs = uint64(8192)
	maxProcs := minHostUIDProcs
	if spec.MaxPIDs > 0 && uint64(spec.MaxPIDs) > minHostUIDProcs {
		maxProcs = uint64(spec.MaxPIDs)
	}
	var rlim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NPROC, &rlim); err == nil {
		if maxProcs < rlim.Cur || rlim.Cur == unix.RLIM_INFINITY {
			rlim.Cur = maxProcs
			_ = unix.Setrlimit(unix.RLIMIT_NPROC, &rlim)
		}
	}

	// 2. CPU Timeout: Enforce hard execution seconds
	if spec.Timeout > 0 {
		var cpuRlim unix.Rlimit
		if err := unix.Getrlimit(unix.RLIMIT_CPU, &cpuRlim); err == nil {
			cpuSecs := uint64(spec.Timeout.Seconds()) + 1
			if cpuSecs < cpuRlim.Cur || cpuRlim.Cur == unix.RLIM_INFINITY {
				cpuRlim.Cur = cpuSecs
				_ = unix.Setrlimit(unix.RLIMIT_CPU, &cpuRlim)
			}
		}
	}

	return nil
}

// Exec executes a sandboxed child command.
func (d *LandlockDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	start := time.Now()

	var isHomeOrRoot bool
	cleanCwd := ""
	cleanHome := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cleanHome = filepath.Clean(home)
	}
	if spec.Cwd != "" {
		cleanCwd = filepath.Clean(spec.Cwd)
		if cleanCwd == "/" || (cleanHome != "" && cleanCwd == cleanHome) {
			isHomeOrRoot = true
		}
	}
	if isHomeOrRoot && os.Getenv(InsideEnvVar) != "1" {
		if cleanCwd == "/" {
			fmt.Fprintln(os.Stderr, "sandbox: working directory is /; isolated to script file only. No other files in / are accessible.")
		} else {
			fmt.Fprintln(os.Stderr, "sandbox: working directory is $HOME; isolated to script file only. No other files in $HOME are accessible.")
		}
	}

	targetBin := spec.Command[0]
	if !filepath.IsAbs(targetBin) {
		if resolved, lookErr := exec.LookPath(targetBin); lookErr == nil {
			targetBin = resolved
		}
	}

	baseEnv := os.Environ()
	if len(spec.Env) > 0 {
		baseEnv = append([]string(nil), spec.Env...)
	}

	if os.Getenv(InsideEnvVar) == "1" {
		if policyErr := checkContainerExecPolicy(spec); policyErr != nil {
			return &ExecResult{
				Duration: time.Since(start),
				Stderr:   policyErr.Error() + "\n",
				ExitCode: 126,
			}, nil
		}
	}

	// When not already inside a sandbox and Landlock is supported by the host kernel,
	// use self-reexec so the child process applies full in-process Landlock + Seccomp-BPF
	// confinement before executing the target binary.
	useSelfReexec := os.Getenv(InsideEnvVar) == "" && d.Available() && os.Getenv(landlockExecEnvVar) == ""
	var selfBin string
	var reexecEnv []string
	if useSelfReexec {
		if exe, exeErr := os.Executable(); exeErr == nil {
			payloadBytes, jsonErr := json.Marshal(landlockExecPayload{
				TargetBin:        targetBin,
				TargetArgv:       spec.Command,
				Cwd:              spec.Cwd,
				Network:          spec.Network,
				NetworkAllow:     spec.NetworkAllow,
				NetworkDeny:      spec.NetworkDeny,
				Mounts:           spec.Mounts,
				MaxMemoryMB:      spec.MaxMemoryMB,
				MaxPIDs:          spec.MaxPIDs,
				Timeout:          spec.Timeout,
				ScriptFile:       spec.ScriptFile,
				NoExec:           spec.NoExec,
				AllowedExecPaths: spec.AllowedExecPaths,
				AuditLog:         spec.AuditLog,
			})
			if jsonErr == nil {
				selfBin = exe
				reexecEnv = append(append([]string(nil), baseEnv...), fmt.Sprintf("%s=%s", landlockExecEnvVar, string(payloadBytes)))
			} else {
				useSelfReexec = false
			}
		} else {
			useSelfReexec = false
		}
	}

	runAttempt := func(withUserNS bool) (*ExecResult, error) {
		execCtx := ctx
		var cancel context.CancelFunc
		if spec.Timeout > 0 {
			execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
			defer cancel()
		}

		var cmd *exec.Cmd
		if useSelfReexec {
			cmd = exec.CommandContext(execCtx, selfBin)
			cmd.Env = reexecEnv
		} else {
			cmd = exec.CommandContext(execCtx, spec.Command[0], spec.Command[1:]...)
			cmd.Env = baseEnv
		}

		if spec.Cwd != "" {
			cmd.Dir = spec.Cwd
		}

		if withUserNS {
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

		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
		cmd.SysProcAttr.Setpgid = true

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
			ExitCode: -1,
		}

		if execCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = 124
			return result, fmt.Errorf("sandbox: command timed out after %v", spec.Timeout)
		}

		if cmd.ProcessState != nil {
			result.ExitCode = cmd.ProcessState.ExitCode()
		}

		return result, err
	}

	wantUserNS := spec.Network == NetworkNone || spec.Network == NetworkLoopback
	result, err := runAttempt(wantUserNS)
	if err != nil && wantUserNS && useSelfReexec {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) &&
			(errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) || errors.Is(err, os.ErrPermission)) {
			// When host kernel restricts unprivileged userns (e.g. Ubuntu 24.04 AppArmor),
			// self-reexec still enforces NetworkNone/NetworkLoopback via Seccomp-BPF inside the child.
			result, err = runAttempt(false)
		}
	}

	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Stderr = enrichPermissionStderr(result.Stderr, spec)
			return result, nil
		}
		if wantUserNS && (errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) || errors.Is(err, os.ErrPermission)) {
			return result, fmt.Errorf("sandbox: network isolation (network=%q) failed: %w (unprivileged user namespaces restricted or unsupported by host kernel; use --sandbox-driver=podman or configure kernel.apparmor_restrict_unprivileged_userns=0)", spec.Network, err)
		}
		return result, FormatPermissionError(fmt.Errorf("sandbox: landlock exec failed: %w", err), targetBin, spec)
	}

	if result.ExitCode != 0 {
		result.Stderr = enrichPermissionStderr(result.Stderr, spec)
	}

	return result, nil
}

func addLandlockPathRule(rulesetFd int, path string, access uint64) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		// On non-directory inodes (regular files, character devices such as /dev/null),
		// the kernel only permits file-level access flags and returns EINVAL if any
		// directory-level flag (such as RemoveFile or ReadDir) is set.
		fileValidFlags := uint64(
			landlockAccessFsExecute |
				landlockAccessFsWriteFile |
				landlockAccessFsReadFile |
				landlockAccessFsTruncate |
				landlockAccessFsIoctlDev,
		)
		access &= fileValidFlags
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
