package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/project-starkite/starkite/libkite"
)

// Win32 and Winsock error numbers defined portably for cross-platform
// error classification and unit testing.
const (
	winErrorAccessDenied     syscall.Errno = 5     // ERROR_ACCESS_DENIED
	winErrorInvalidAccess    syscall.Errno = 12    // ERROR_INVALID_ACCESS
	winErrorSharingViolation syscall.Errno = 32    // ERROR_SHARING_VIOLATION
	winErrorPrivilegeNotHeld syscall.Errno = 1314  // ERROR_PRIVILEGE_NOT_HELD
	winErrorNotEnoughQuota   syscall.Errno = 1816  // ERROR_NOT_ENOUGH_QUOTA
	wsaErrorAccessDenied     syscall.Errno = 10013 // WSAEACCES
	wsaErrorAddrNotAvail     syscall.Errno = 10049 // WSAEADDRNOTAVAIL
	wsaErrorNetUnreach       syscall.Errno = 10051 // WSAENETUNREACH
	wsaErrorHostUnreach      syscall.Errno = 10065 // WSAEHOSTUNREACH
)

func isOSPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, winErrorAccessDenied) ||
		errors.Is(err, winErrorInvalidAccess) ||
		errors.Is(err, winErrorSharingViolation) ||
		errors.Is(err, winErrorPrivilegeNotHeld) ||
		errors.Is(err, winErrorNotEnoughQuota) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "operation not permitted") ||
		strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "a required privilege is not held") ||
		strings.Contains(lower, "not enough quota is available") ||
		strings.Contains(lower, "unauthorizedaccessexception")
}

func isOSNetworkDeniedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, winErrorAccessDenied) ||
		errors.Is(err, wsaErrorAccessDenied) ||
		errors.Is(err, wsaErrorAddrNotAvail) ||
		errors.Is(err, wsaErrorNetUnreach) ||
		errors.Is(err, wsaErrorHostUnreach) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "operation not permitted") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "network is unreachable") ||
		strings.Contains(lower, "forbidden by its access permissions") ||
		strings.Contains(lower, "wsaeacces") ||
		strings.Contains(lower, "no route to host")
}

func isQuotaExceededError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, winErrorNotEnoughQuota) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "not enough quota")
}

func stripExtendedPathPrefix(p string) string {
	if strings.HasPrefix(p, `\\?\`) || strings.HasPrefix(p, `\??\`) {
		return p[4:]
	}
	return p
}

func normalizeDiagPath(p string) string {
	raw := stripExtendedPathPrefix(strings.TrimSpace(p))
	// Convert backslashes to forward slashes and ensure separators are forward slashes
	// consistently across both Unix and Windows hosts.
	slash := strings.ReplaceAll(raw, "\\", "/")
	return filepath.ToSlash(filepath.Clean(slash))
}

func equalOrHasPathPrefixFold(target, prefix string) bool {
	if prefix == "" || target == "" {
		return false
	}
	targetSlash := filepath.ToSlash(target)
	prefixSlash := filepath.ToSlash(prefix)
	if strings.EqualFold(targetSlash, prefixSlash) {
		return true
	}
	slashPrefix := prefixSlash
	if !strings.HasSuffix(slashPrefix, "/") {
		slashPrefix += "/"
	}
	return len(targetSlash) > len(prefixSlash) && strings.EqualFold(targetSlash[:len(slashPrefix)], slashPrefix)
}

func isScratchPath(cleanTarget string) bool {
	slash := filepath.ToSlash(cleanTarget)
	if slash == "/tmp" || strings.HasPrefix(slash, "/tmp/") ||
		slash == "/private/tmp" || strings.HasPrefix(slash, "/private/tmp/") {
		return true
	}
	if strings.Contains(strings.ToLower(slash), "/starkite-sbx-") {
		return true
	}
	return false
}

func trimExecutableExt(name string) string {
	lower := strings.ToLower(name)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".com"} {
		if strings.HasSuffix(lower, ext) {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}

// FormatPermissionError wraps OS-level EACCES/EPERM/Win32 errors with actionable
// sandbox policy attribution and CLI/config remediation hints explaining why
// the operation was denied and how to grant the required capability.
func FormatPermissionError(err error, targetPath string, spec *ExecutionSpec) error {
	if err == nil {
		return nil
	}
	if !isOSPermissionError(err) {
		return err
	}
	if spec == nil {
		if targetPath != "" {
			hint := libkite.SuggestCapabilityHint("fs", "read", "", targetPath)
			return fmt.Errorf("sandbox: permission denied accessing %q: %w\n  Hint: %s", targetPath, err, hint)
		}
		return fmt.Errorf("sandbox: permission denied: %w", err)
	}

	if isQuotaExceededError(err) {
		if spec.NoExec {
			return fmt.Errorf("sandbox: subprocess execution denied by no_exec policy (active process limit): %w\n  Hint: pass --sandbox-exec-allow=* on CLI, or add \"*\" to sandbox.<profile>.exec.allow in config.yaml", err)
		}
		return fmt.Errorf("sandbox: process resource limit exceeded (quota exhausted): %w\n  Hint: adjust memory or process limits in your sandbox profile or flags", err)
	}

	if spec.NoExec && targetPath == "" {
		return fmt.Errorf("sandbox: subprocess execution denied by no_exec policy: %w\n  Hint: pass --sandbox-exec-allow=* on CLI, or add \"*\" to sandbox.<profile>.exec.allow in config.yaml", err)
	}

	if targetPath != "" {
		cleanTarget := normalizeDiagPath(targetPath)
		if cleanTarget == "/dev/shm" || strings.HasPrefix(cleanTarget, "/dev/shm/") ||
			cleanTarget == "/run/shm" || strings.HasPrefix(cleanTarget, "/run/shm/") {
			return fmt.Errorf("sandbox: access to %q denied by shared memory (/dev/shm) isolation: %w\n  Hint: shared memory segments (/dev/shm) are isolated by default; use /tmp for scratch files", targetPath, err)
		}
		if isScratchPath(cleanTarget) {
			suggestBin := trimExecutableExt(filepath.Base(cleanTarget))
			return fmt.Errorf("sandbox: execution or access of %q denied on writable scratch mount (W^X noexec policy): %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or place executables on a read-only path", targetPath, err, suggestBin)
		}
		for _, m := range spec.Mounts {
			dst := normalizeDiagPath(m.Destination)
			src := normalizeDiagPath(m.Source)
			if (m.Mode == MountRW || m.Type == MountTmpfs) &&
				(equalOrHasPathPrefixFold(cleanTarget, dst) || (src != "" && equalOrHasPathPrefixFold(cleanTarget, src))) {
				suggestBin := trimExecutableExt(filepath.Base(cleanTarget))
				return fmt.Errorf("sandbox: execution of %q denied on writable mount %q (W^X noexec policy): %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or mount %q read-only (--sandbox-path-read)", targetPath, m.Destination, err, suggestBin, m.Destination)
			}
		}

		userHome := os.Getenv("USERPROFILE")
		if userHome == "" {
			userHome, _ = os.UserHomeDir()
		}
		if userHome == "" {
			userHome = os.Getenv("HOME")
		}
		if userHome != "" {
			cleanHome := normalizeDiagPath(userHome)
			if equalOrHasPathPrefixFold(cleanTarget, cleanHome) {
				return fmt.Errorf("sandbox: access to %q denied by home directory isolation ($HOME is not mounted by default; add an explicit mount in your sandbox profile): %w\n  Hint: pass --sandbox-path-read=%s on CLI, or add %q to sandbox.<profile>.paths.read in config.yaml", targetPath, err, cleanTarget, cleanTarget)
			}
		}

		if spec.NoExec {
			suggestBin := trimExecutableExt(filepath.Base(cleanTarget))
			return fmt.Errorf("sandbox: execution of %q denied by no_exec policy: %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or add %q to sandbox.<profile>.exec.allow in config.yaml", targetPath, err, suggestBin, suggestBin)
		}
		if len(spec.AllowedExecPaths) > 0 {
			suggestBin := trimExecutableExt(filepath.Base(cleanTarget))
			return fmt.Errorf("sandbox: execution of %q denied by allowed_exec_paths policy: %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or add %q to sandbox.<profile>.exec.allow in config.yaml", targetPath, err, suggestBin, suggestBin)
		}
		hint := libkite.SuggestCapabilityHint("fs", "read", "", targetPath)
		return fmt.Errorf("sandbox: access to %q denied by sandbox policy (path is outside active mounts or read-only): %w\n  Hint: %s", targetPath, err, hint)
	}
	return fmt.Errorf("sandbox: operation denied by sandbox policy: %w", err)
}

// FormatNetworkError wraps outbound network socket or connection errors under
// network isolation with an actionable remediation hint.
func FormatNetworkError(err error, endpoint string, spec *ExecutionSpec) error {
	if err == nil {
		return nil
	}
	if !isOSNetworkDeniedError(err) {
		return err
	}
	mode := NetworkNone
	if spec != nil && spec.Network != "" {
		mode = spec.Network
	}
	hostLabel := endpoint
	if hostLabel == "" {
		hostLabel = "outbound endpoint"
	}
	if endpoint != "" && endpoint != "*" {
		return fmt.Errorf(
			"sandbox: outbound network socket creation denied for host %q (sandbox network: %s): %w\n  Hint: pass --sandbox-net-allow=%s on CLI, or add %q to sandbox.<profile>.network.allow in config.yaml",
			hostLabel, mode, err, endpoint, endpoint,
		)
	}
	return fmt.Errorf(
		"sandbox: outbound network socket creation denied for host %q (sandbox network: %s): %w\n  Hint: pass --sandbox-net-allow on CLI, or set sandbox.<profile>.network.mode: host in config.yaml",
		hostLabel, mode, err,
	)
}

// FormatEnvScrubbedDiagnostic formats a debug warning when a script queries an
// environment variable that exists on the host but was scrubbed by zero-trust
// isolation.
func FormatEnvScrubbedDiagnostic(varName string) string {
	return libkite.FormatEnvScrubbedDiagnostic(varName)
}

// extractCommandTarget extracts the probable target executable from command tokens,
// skipping shell wrappers such as /bin/sh, cmd.exe, and powershell.exe.
func extractCommandTarget(cmd []string) string {
	target := ""
	for i := range cmd {
		arg := cmd[i]
		tokens := strings.Fields(arg)
		for j, tok := range tokens {
			cleanTok := strings.Trim(tok, "'\"`;|&()")
			lowerTok := strings.ToLower(cleanTok)
			baseName := strings.ToLower(filepath.Base(cleanTok))

			// Skip shell wrappers
			if baseName == "sh" || baseName == "bash" || baseName == "zsh" ||
				baseName == "env" || baseName == "cmd.exe" || baseName == "cmd" ||
				baseName == "powershell.exe" || baseName == "powershell" ||
				baseName == "pwsh.exe" || baseName == "pwsh" {
				continue
			}
			// Skip shell flags
			if strings.HasPrefix(cleanTok, "-") || strings.HasPrefix(cleanTok, "/") && len(cleanTok) == 2 {
				continue
			}

			// Path-qualified or explicit executable extension
			hasSlash := strings.Contains(cleanTok, "/") || strings.Contains(cleanTok, "\\")
			hasExt := strings.HasSuffix(lowerTok, ".exe") || strings.HasSuffix(lowerTok, ".cmd") ||
				strings.HasSuffix(lowerTok, ".bat") || strings.HasSuffix(lowerTok, ".ps1") ||
				strings.HasSuffix(lowerTok, ".sh")

			if hasSlash || hasExt {
				target = cleanTok
				break
			}
			if target == "" && j == 0 && !strings.Contains(cleanTok, "=") {
				target = cleanTok
			}
		}
		if target != "" {
			break
		}
	}
	return target
}

// enrichPermissionStderr appends actionable sandbox policy attribution from
// FormatPermissionError or FormatNetworkError when a sandboxed process fails
// with a permission error.
func enrichPermissionStderr(stderr string, spec *ExecutionSpec) string {
	if spec == nil {
		return stderr
	}
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "sandbox:") {
		return stderr
	}
	if strings.Contains(lower, "unknown flag") ||
		strings.Contains(lower, "cannot connect to the docker") ||
		strings.Contains(lower, "cannot connect to podman") ||
		strings.Contains(lower, "unable to connect to podman") {
		return stderr
	}

	netDenied := (spec.Network == NetworkNone || spec.Network == NetworkLoopback) &&
		(strings.Contains(lower, "socket:") ||
			strings.Contains(lower, "network is unreachable") ||
			strings.Contains(lower, "dial tcp") ||
			strings.Contains(lower, "dial udp") ||
			strings.Contains(lower, "forbidden by its access permissions") ||
			strings.Contains(lower, "wsaeacces") ||
			strings.Contains(lower, "10013") ||
			strings.Contains(lower, "failed to connect") ||
			strings.Contains(lower, "could not resolve host")) &&
		(strings.Contains(lower, "operation not permitted") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "network is unreachable") ||
			strings.Contains(lower, "access is denied") ||
			strings.Contains(lower, "forbidden by its access permissions") ||
			strings.Contains(lower, "could not resolve host") ||
			strings.Contains(lower, "failed to connect") ||
			strings.Contains(lower, "10013") ||
			strings.Contains(lower, "wsaeacces"))

	if netDenied {
		diag := FormatNetworkError(syscall.EPERM, "", spec).Error()
		if stderr == "" {
			return diag + "\n"
		}
		if !strings.HasSuffix(stderr, "\n") {
			stderr += "\n"
		}
		return stderr + diag + "\n"
	}

	hasPermString := strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "operation not permitted") ||
		strings.Contains(lower, "not permitted") ||
		strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "unauthorizedaccessexception") ||
		strings.Contains(lower, "a required privilege is not held") ||
		strings.Contains(lower, "not enough quota is available")

	if !hasPermString && !spec.NoExec && len(spec.AllowedExecPaths) == 0 {
		return stderr
	}

	target := extractCommandTarget(spec.Command)
	var errToFormat error = syscall.EACCES
	if strings.Contains(lower, "not enough quota") {
		errToFormat = winErrorNotEnoughQuota
	}
	diag := FormatPermissionError(errToFormat, target, spec).Error()
	if stderr == "" {
		return diag + "\n"
	}
	if !strings.HasSuffix(stderr, "\n") {
		stderr += "\n"
	}
	return stderr + diag + "\n"
}

// EnrichExecDiagnostics inspects an ExecResult for failed commands and appends
// actionable sandbox remediation hints to res.Stderr for common denial patterns
// such as missing language runtime dependencies or network isolation.
func EnrichExecDiagnostics(res *ExecResult, spec *ExecutionSpec) {
	if res == nil || res.ExitCode == 0 {
		return
	}

	lower := strings.ToLower(res.Stderr)

	if strings.Contains(lower, "modulenotfounderror: no module named") {
		hint := "Hint: pass --sandbox-profile=python-env on CLI, or add dependencies via --sandbox-path-read=<path>"
		if !strings.Contains(res.Stderr, hint) {
			if res.Stderr != "" && !strings.HasSuffix(res.Stderr, "\n") {
				res.Stderr += "\n"
			}
			res.Stderr += hint + "\n"
		}
	}

	if strings.Contains(lower, "cannot find module") {
		hint := "Hint: pass --sandbox-profile=node-env on CLI, or grant access via --sandbox-path-read=node_modules"
		if !strings.Contains(res.Stderr, hint) {
			if res.Stderr != "" && !strings.HasSuffix(res.Stderr, "\n") {
				res.Stderr += "\n"
			}
			res.Stderr += hint + "\n"
		}
	}

	if strings.Contains(lower, "cannot load such file") || strings.Contains(lower, "loaderror") {
		hint := "Hint: pass --sandbox-profile=ruby-env on CLI, or add dependencies via --sandbox-path-read=<path>"
		if !strings.Contains(res.Stderr, hint) {
			if res.Stderr != "" && !strings.HasSuffix(res.Stderr, "\n") {
				res.Stderr += "\n"
			}
			res.Stderr += hint + "\n"
		}
	}

	isDyldFailure := strings.Contains(lower, "image not found") && (strings.Contains(lower, "dyld") || strings.Contains(lower, "library not loaded"))
	if strings.Contains(lower, "cannot open shared object file") || isDyldFailure {
		hint := "Hint: grant read access to library location via --sandbox-path-read=<dir>"
		if !strings.Contains(res.Stderr, hint) {
			if res.Stderr != "" && !strings.HasSuffix(res.Stderr, "\n") {
				res.Stderr += "\n"
			}
			res.Stderr += hint + "\n"
		}
	}

	if spec != nil && (spec.Network == NetworkNone || len(spec.NetworkDeny) > 0) {
		if strings.Contains(lower, "failed to connect") || strings.Contains(lower, "could not resolve host") {
			hint := "Hint: pass --sandbox-net-allow on CLI, or add endpoint to sandbox.<profile>.network.allow in config.yaml"
			if !strings.Contains(res.Stderr, hint) {
				if res.Stderr != "" && !strings.HasSuffix(res.Stderr, "\n") {
					res.Stderr += "\n"
				}
				res.Stderr += hint + "\n"
			}
		}
	}
}

// PolicySummary constructs a libkite.SandboxPolicySummary from the resolved
// Profile and CLI environment flags so the runtime and module loader can
// enforce module trust boundary non-escalation.
func (p Profile) PolicySummary(cliEnvFlags []string, cliEnvInherit bool, displayPath string) *libkite.SandboxPolicySummary {
	profName := p.Name
	if profName == "" {
		profName = DefaultProfileName
	}
	netMode := string(p.Network)
	if netMode == "" {
		if p.Enabled {
			netMode = string(NetworkNone)
		} else {
			netMode = string(NetworkHost)
		}
	}
	envAllowed := make(map[string]bool)
	for k := range p.Env.Vars {
		envAllowed[k] = true
	}
	for _, entry := range cliEnvFlags {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if key != "" {
			envAllowed[key] = true
		}
	}
	return &libkite.SandboxPolicySummary{
		ProfileName: profName,
		Enabled:     p.Enabled,
		Network:     netMode,
		PathsRead:   slices.Clone(p.Paths.Read),
		PathsWrite:  slices.Clone(p.Paths.Write),
		ExecAllow:   slices.Clone(p.Exec.Allow),
		ExecDeny:    slices.Clone(p.Exec.Deny),
		CallsAllow:  slices.Clone(p.Calls.Allow),
		CallsDeny:   slices.Clone(p.Calls.Deny),
		EnvInherit:  p.Env.Inherit || cliEnvInherit,
		EnvAllowed:  envAllowed,
		DisplayPath: displayPath,
	}
}
