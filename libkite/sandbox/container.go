package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// Standard container engine names.
const (
	EnginePodman  = "podman"
	EngineDocker  = "docker"
	EngineNerdctl = "nerdctl"

	// DefaultContainerImage is the fallback base image if none is specified in ExecutionSpec.
	DefaultContainerImage = "ghcr.io/project-starkite/starkite:latest"
)

// ContainerDriver implements the Driver interface by orchestrating ephemeral
// containers via the Podman, Docker, or Nerdctl CLI.
type ContainerDriver struct {
	mu      sync.Mutex
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

func (d *ContainerDriver) resolveBinary() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.binPath == "" {
		d.binPath, _ = exec.LookPath(d.engine)
	}
	return d.binPath
}

// Available reports whether the container CLI binary is installed and executable on the host.
func (d *ContainerDriver) Available() bool {
	return d.resolveBinary() != ""
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
	var args []string
	// Rootless Podman delegates runsc cgroups via systemd by default, which fails in
	// non-interactive sessions unless cgroupfs is selected.
	if d.isRootlessPodman() && spec.Runtime == BinRunsc {
		args = append(args, "--cgroup-manager=cgroupfs")
	}
	args = append(args, "run", "--rm", "-i")

	// 7-Point Container Hardening: drop ambient capabilities, prevent privilege escalation,
	// enforce private IPC namespace, and lock container rootfs to read-only.
	args = append(args,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--ipc=private",
		"--read-only",
	)

	// User mapping: on Linux hosts, run as the invoking user/group to prevent root execution
	// and avoid file ownership corruption on host mounts.
	if runtime.GOOS == "linux" {
		if d.isRootlessPodman() {
			args = append(args, "--userns=keep-id")
		}
		args = append(args, fmt.Sprintf("--user=%d:%d", os.Getuid(), os.Getgid()))
	}

	// Network configuration
	switch spec.Network {
	case NetworkHost:
		// Under rootless Podman with gVisor (runsc), --network=host leaves runsc's
		// userspace netstack with zero interfaces (not even lo / 127.0.0.1). Omitting
		// --network=host uses Podman's rootless pasta/slirp4netns namespace, giving
		// runsc both a working loopback interface and outbound network connectivity.
		if !(d.isRootlessPodman() && spec.Runtime == BinRunsc) {
			args = append(args, "--network=host")
		}
	case NetworkNone, NetworkLoopback, "":
		args = append(args, "--network=none")
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

	// Anti-fork-bomb barrier: default to conservative 256 PIDs if unconstrained
	pidsLimit := spec.MaxPIDs
	if pidsLimit <= 0 {
		pidsLimit = 256
	}
	args = append(args, fmt.Sprintf("--pids-limit=%d", pidsLimit))

	// Container parity: if Cwd is $HOME or /, omit host $HOME / / from writable mounts
	isMatch, _ := isHomeOrRoot(spec.Cwd)
	var cleanHome string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cleanHome = filepath.ToSlash(filepath.Clean(home))
	}

	hasTmp := false
	var mountedDsts []string

	// Mount bindings
	for _, m := range spec.Mounts {
		cleanDst := filepath.ToSlash(filepath.Clean(m.Destination))
		if cleanDst == "/tmp" {
			hasTmp = true
		}

		// Isolate shared memory (/dev/shm and /run/shm): container runs with --ipc=private,
		// so omit any host /dev/shm or /run/shm bind/tmpfs overrides.
		if cleanDst == "/dev/shm" || strings.HasPrefix(cleanDst, "/dev/shm/") ||
			cleanDst == "/run/shm" || strings.HasPrefix(cleanDst, "/run/shm/") {
			continue
		}

		if m.Type == MountTmpfs {
			args = append(args, fmt.Sprintf("--tmpfs=%s:rw,noexec,nosuid,nodev", m.Destination))
			mountedDsts = append(mountedDsts, cleanDst)
			continue
		}

		src := m.Source
		if src == "" {
			src = m.Destination
		}

		cleanSrc := filepath.ToSlash(filepath.Clean(src))
		if cleanSrc == "/dev/shm" || strings.HasPrefix(cleanSrc, "/dev/shm/") ||
			cleanSrc == "/run/shm" || strings.HasPrefix(cleanSrc, "/run/shm/") {
			continue
		}

		// Under container parity, if Cwd == $HOME or /, omit host $HOME or / bind mount
		if isMatch && (cleanSrc == cleanHome || isRootPath(cleanSrc) || cleanDst == cleanHome || isRootPath(cleanDst)) {
			continue
		}

		// Skip host system directories on all OSes so the Alpine/musl container retains
		// its native runtime, entrypoint, libraries, TLS CA certs, and network resolver.
		if isContainerSystemPath(cleanDst) {
			continue
		}

		mode := "ro"
		if m.Mode == MountRW {
			mode = "rw"
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", src, m.Destination, mode))
		mountedDsts = append(mountedDsts, cleanDst)
	}

	// Always ensure an isolated, ephemeral /tmp tmpfs is mounted for scratch space
	if !hasTmp {
		args = append(args, "--tmpfs=/tmp:rw,noexec,nosuid,nodev")
		mountedDsts = append(mountedDsts, "/tmp")
	}

	if spec.ScriptFile != "" {
		scriptPath := spec.ScriptFile
		if !filepath.IsAbs(scriptPath) && spec.Cwd != "" {
			scriptPath = filepath.Join(spec.Cwd, scriptPath)
		}
		cleanScript := filepath.Clean(scriptPath)
		slashScript := filepath.ToSlash(cleanScript)
		if !isContainerSystemPath(slashScript) && !isCoveredByMount(slashScript, mountedDsts) {
			args = append(args, "-v", fmt.Sprintf("%s:%s:ro", cleanScript, cleanScript))
			mountedDsts = append(mountedDsts, slashScript)
		}
	}

	if spec.Cwd != "" && containerPathExists(spec.Cwd, mountedDsts) {
		args = append(args, "--workdir", spec.Cwd)
	}

	// Container Image
	image := spec.Image
	if image == "" {
		image = DefaultContainerImage
	}

	// Command assembly
	command := spec.Command
	if len(command) > 0 {
		binName := strings.TrimSuffix(strings.ToLower(filepath.Base(command[0])), ".exe")
		isKiteBin := binName == "kite" || binName == "kitecmd" || binName == "kitecloud" || binName == "kiteai"

		// If the container image is the official Starkite image (built with ko or OCI entrypoint "kite"),
		// the image already defines the entrypoint as /usr/local/bin/kite (/ko-app/kite).
		// Strip kite binary name for self-handoff, or clear entrypoint for direct native commands.
		if isKiteImage(image) {
			if isKiteBin {
				command = command[1:]
			} else {
				args = append(args, "--entrypoint=")
			}
		} else if runtime.GOOS == "linux" && (image == "docker.io/library/alpine:latest" || image == "alpine:latest" || image == "alpine") {
			// If host is Linux and a minimal base image (alpine) is used, bind-mount the host Linux binary
			// when it is not a standard system binary already present in the container rootfs.
			if filepath.IsAbs(command[0]) {
				binPath := filepath.Clean(command[0])
				isSystemBin := strings.HasPrefix(binPath, "/bin/") ||
					strings.HasPrefix(binPath, "/sbin/") ||
					strings.HasPrefix(binPath, "/usr/bin/") ||
					strings.HasPrefix(binPath, "/usr/sbin/")
				if !isSystemBin {
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
			}
		} else if isKiteBin {
			// Non-starkite container on macOS/Windows or custom container image with kite installed in /usr/local/bin
			command = append([]string{"/usr/local/bin/kite"}, command[1:]...)
		}
	}

	// Custom runtime override (e.g. --runtime=runsc)
	if spec.Runtime != "" {
		args = append(args, fmt.Sprintf("--runtime=%s", spec.Runtime))
	}

	args = append(args, image)
	args = append(args, command...)

	return args
}

// isKiteImage reports whether the given image reference represents the Starkite CLI container image.
func isKiteImage(img string) bool {
	if img == "" || img == DefaultContainerImage {
		return true
	}
	clean := strings.ToLower(img)
	if beforeDigest, _, hasDigest := strings.Cut(clean, "@"); hasDigest {
		clean = beforeDigest
	}
	lastSlash := strings.LastIndex(clean, "/")
	repoName := clean
	if lastSlash >= 0 {
		repoName = clean[lastSlash+1:]
	}
	if beforeTag, _, hasTag := strings.Cut(repoName, ":"); hasTag {
		repoName = beforeTag
	}
	return repoName == "starkite" || repoName == "kite"
}

func resolveContainerExecTarget(cmdBin, cwd string) string {
	if cmdBin == "" {
		return ""
	}
	resolved := cmdBin
	if !isHostAbs(resolved) && cwd != "" && (strings.ContainsRune(resolved, filepath.Separator) || strings.Contains(resolved, "/") || strings.HasPrefix(resolved, ".")) {
		resolved = filepath.Join(cwd, resolved)
	} else if !strings.ContainsAny(resolved, `/\`) {
		if lookedUp, err := exec.LookPath(resolved); err == nil {
			resolved = lookedUp
		}
	}
	if canonical, err := filepath.EvalSymlinks(resolved); err == nil && canonical != "" {
		resolved = canonical
	}
	return filepath.Clean(resolved)
}

func isPathInsideWritableMount(candidate string, mounts []Mount) bool {
	if candidate == "" {
		return false
	}
	cleanCand := filepath.ToSlash(filepath.Clean(candidate))
	candTargets := []string{cleanCand}
	if canon, err := filepath.EvalSymlinks(candidate); err == nil && canon != "" {
		candTargets = append(candTargets, filepath.ToSlash(filepath.Clean(canon)))
	}
	for _, c := range candTargets {
		if c == "/tmp" || strings.HasPrefix(c, "/tmp/") ||
			c == "/private/tmp" || strings.HasPrefix(c, "/private/tmp/") {
			return true
		}
		for _, m := range mounts {
			if m.Mode == MountRW || m.Type == MountTmpfs {
				for _, mountPath := range []string{m.Destination, m.Source} {
					if mountPath == "" {
						continue
					}
					cleanMount := filepath.ToSlash(filepath.Clean(mountPath))
					if c == cleanMount || strings.HasPrefix(c, cleanMount+"/") {
						return true
					}
					if canonMount, err := filepath.EvalSymlinks(mountPath); err == nil && canonMount != "" {
						cleanCanonMount := filepath.ToSlash(filepath.Clean(canonMount))
						if c == cleanCanonMount || strings.HasPrefix(c, cleanCanonMount+"/") {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// checkContainerExecPolicy enforces NoExec, AllowedExecPaths, and W^X on writable mounts
// for containerized execution parity with Landlock and Seatbelt.
func checkContainerExecPolicy(spec *ExecutionSpec) error {
	if len(spec.Command) == 0 || spec.Command[0] == "" {
		return nil
	}
	if spec.NoExec {
		return FormatPermissionError(os.ErrPermission, spec.Command[0], spec)
	}
	cmdBin := spec.Command[0]
	resolvedBin := resolveContainerExecTarget(cmdBin, spec.Cwd)

	if len(spec.AllowedExecPaths) > 0 {
		if !isAllowedExecTarget(resolvedBin, spec.Cwd, spec.AllowedExecPaths) &&
			!isAllowedExecTarget(cmdBin, spec.Cwd, spec.AllowedExecPaths) {
			return FormatPermissionError(os.ErrPermission, cmdBin, spec)
		}
	}

	// Enforce W^X on direct binary or interpreter script targets inside writable mounts or /tmp.
	if isPathInsideWritableMount(resolvedBin, spec.Mounts) {
		return FormatPermissionError(os.ErrPermission, cmdBin, spec)
	}
	baseCmd := strings.TrimSuffix(strings.ToLower(filepath.Base(resolvedBin)), ".exe")
	switch baseCmd {
	case "sh", "bash", "dash", "zsh", "python", "python3", "perl", "ruby", "node":
		for _, arg := range spec.Command[1:] {
			if arg == "" || strings.HasPrefix(arg, "-") {
				continue
			}
			if !strings.ContainsAny(arg, `/\`) && !strings.HasPrefix(arg, ".") {
				continue
			}
			resolvedArg := resolveContainerExecTarget(arg, spec.Cwd)
			if isPathInsideWritableMount(resolvedArg, spec.Mounts) {
				return FormatPermissionError(os.ErrPermission, arg, spec)
			}
		}
	}
	return nil
}

func newEphemeralContainerName() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("starkite-sbx-%d", time.Now().UnixNano())
	}
	return "starkite-sbx-" + hex.EncodeToString(b[:])
}

// Exec executes the command inside an ephemeral container using the container engine CLI.
func (d *ContainerDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	if !spec.SelfHandoff {
		if policyErr := checkContainerExecPolicy(spec); policyErr != nil {
			return policyDeniedResult(spec, policyErr), nil
		}
	}

	binPath := d.resolveBinary()
	if binPath == "" {
		return nil, fmt.Errorf("sandbox: container engine %q CLI binary not found in PATH", d.engine)
	}

	start := time.Now()
	cliArgs := d.BuildArgs(spec)
	containerName := newEphemeralContainerName()
	if runIdx := slices.Index(cliArgs, "run"); runIdx >= 0 {
		cliArgs = slices.Insert(cliArgs, runIdx+1, "--name", containerName)
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	// Under container parity, emit notification if Cwd is $HOME or /
	if isMatch, isRoot := isHomeOrRoot(spec.Cwd); isMatch {
		msg := "sandbox (container): working directory is $HOME; isolated to script file only. No other files in $HOME are mounted."
		if isRoot {
			msg = "sandbox (container): working directory is /; isolated to script file only. No other files in / are mounted."
		}
		fmt.Fprintln(os.Stderr, msg)
		if spec.Stderr != nil && spec.Stderr != os.Stderr {
			fmt.Fprintln(spec.Stderr, msg)
		}
	}

	cmd := exec.CommandContext(execCtx, binPath, cliArgs...)
	configureSubprocessCancel(cmd)

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
		// Killing the local container CLI does not guarantee the daemon stops
		// the container, so force-remove it by its ephemeral name.
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 5*time.Second)
		rmArgs := []string{"rm", "-f", containerName}
		if d.isRootlessPodman() && spec.Runtime == BinRunsc {
			rmArgs = append([]string{"--cgroup-manager=cgroupfs"}, rmArgs...)
		}
		_ = exec.CommandContext(rmCtx, binPath, rmArgs...).Run()
		rmCancel()
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
			return result, fmt.Errorf("sandbox: container command timed out after %v: %w", spec.Timeout, execCtx.Err())
		}
		return result, fmt.Errorf("sandbox: container command timed out: %w", execCtx.Err())
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
			// Check for standard OOM exit codes (137 = 128 + SIGKILL)
			if result.ExitCode == 137 && strings.Contains(strings.ToLower(result.Stderr), "oom") {
				result.KilledByOOM = true
			}
			applyPermissionStderr(result, spec)
			return result, nil
		}
		return result, FormatPermissionError(fmt.Errorf("sandbox: container execution failed: %w", err), spec.Command[0], spec)
	}

	applyPermissionStderr(result, spec)

	return result, nil
}

// isHomeOrRoot reports whether the given working directory matches the host user's $HOME or root (/).
// It returns (isMatch, isRoot).
func isHomeOrRoot(cwd string) (bool, bool) {
	if cwd == "" {
		return false, false
	}
	cleanCwd := filepath.ToSlash(filepath.Clean(cwd))
	if isRootPath(cleanCwd) {
		return true, true
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if cleanCwd == filepath.ToSlash(filepath.Clean(home)) {
			return true, false
		}
	}
	return false, false
}

// isRootPath reports whether p represents a filesystem root ("/" or "\" or "C:/" on Windows).
func isRootPath(p string) bool {
	clean := filepath.ToSlash(filepath.Clean(p))
	return clean == "/" || clean == `\` || (len(clean) == 3 && clean[1] == ':' && clean[2] == '/')
}

func (d *ContainerDriver) isRootlessPodman() bool {
	return d.engine == EnginePodman && runtime.GOOS == "linux" && os.Geteuid() != 0
}

func isContainerSystemPath(cleanSlashPath string) bool {
	for _, sysDir := range []string{"/etc", "/usr", "/bin", "/lib", "/lib64", "/sbin"} {
		if cleanSlashPath == sysDir || strings.HasPrefix(cleanSlashPath, sysDir+"/") {
			return true
		}
	}
	return false
}

func isCoveredByMount(cleanSlashPath string, mountedDsts []string) bool {
	for _, dst := range mountedDsts {
		if cleanSlashPath == dst || strings.HasPrefix(cleanSlashPath, dst+"/") {
			return true
		}
	}
	return false
}

func containerPathExists(cwd string, mountedDsts []string) bool {
	cleanCwd := filepath.ToSlash(filepath.Clean(cwd))
	if cleanCwd == "/" || cleanCwd == "/tmp" || strings.HasPrefix(cleanCwd, "/tmp/") {
		return true
	}
	for _, dst := range mountedDsts {
		if cleanCwd == dst || strings.HasPrefix(cleanCwd, dst+"/") || strings.HasPrefix(dst, cleanCwd+"/") {
			return true
		}
	}
	return false
}
