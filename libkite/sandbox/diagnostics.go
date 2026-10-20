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

// FormatPermissionError wraps OS-level EACCES/EPERM errors with actionable
// sandbox policy attribution and CLI/config remediation hints explaining why
// the operation was denied and how to grant the required capability.
func FormatPermissionError(err error, targetPath string, spec *ExecutionSpec) error {
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrPermission) && !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
		return err
	}
	if spec == nil {
		if targetPath != "" {
			hint := libkite.SuggestCapabilityHint("fs", "read", "", targetPath)
			return fmt.Errorf("sandbox: permission denied accessing %q: %w\n  Hint: %s", targetPath, err, hint)
		}
		return fmt.Errorf("sandbox: permission denied: %w", err)
	}
	if spec.NoExec && targetPath == "" {
		return fmt.Errorf("sandbox: subprocess execution denied by no_exec policy: %w\n  Hint: pass --sandbox-exec-allow=* on CLI, or add \"*\" to sandbox.<profile>.exec.allow in config.yaml", err)
	}
	if targetPath != "" {
		cleanTarget := filepath.ToSlash(filepath.Clean(targetPath))
		if cleanTarget == "/dev/shm" || strings.HasPrefix(cleanTarget, "/dev/shm/") ||
			cleanTarget == "/run/shm" || strings.HasPrefix(cleanTarget, "/run/shm/") {
			return fmt.Errorf("sandbox: access to %q denied by shared memory (/dev/shm) isolation: %w\n  Hint: shared memory segments (/dev/shm) are isolated by default; use /tmp for scratch files", targetPath, err)
		}
		if cleanTarget == "/tmp" || strings.HasPrefix(cleanTarget, "/tmp/") ||
			cleanTarget == "/private/tmp" || strings.HasPrefix(cleanTarget, "/private/tmp/") {
			bin := filepath.Base(cleanTarget)
			return fmt.Errorf("sandbox: execution or access of %q denied on writable scratch mount (W^X noexec policy): %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or place executables on a read-only path", targetPath, err, bin)
		}
		for _, m := range spec.Mounts {
			dst := filepath.ToSlash(filepath.Clean(m.Destination))
			src := filepath.ToSlash(filepath.Clean(m.Source))
			if (m.Mode == MountRW || m.Type == MountTmpfs) &&
				(cleanTarget == dst || strings.HasPrefix(cleanTarget, dst+"/") ||
					(m.Source != "" && (cleanTarget == src || strings.HasPrefix(cleanTarget, src+"/")))) {
				bin := filepath.Base(cleanTarget)
				return fmt.Errorf("sandbox: execution of %q denied on writable mount %q (W^X noexec policy): %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or mount %q read-only (--sandbox-path-read)", targetPath, m.Destination, err, bin, m.Destination)
			}
		}
		if home, homeErr := os.UserHomeDir(); homeErr == nil && home != "" {
			cleanHome := filepath.ToSlash(filepath.Clean(home))
			if cleanTarget == cleanHome || strings.HasPrefix(cleanTarget, cleanHome+"/") {
				return fmt.Errorf("sandbox: access to %q denied by home directory isolation ($HOME is not mounted by default; add an explicit mount in your sandbox profile): %w\n  Hint: pass --sandbox-path-read=%s on CLI, or add %q to sandbox.<profile>.paths.read in config.yaml", targetPath, err, cleanTarget, cleanTarget)
			}
		}
		if spec.NoExec {
			bin := filepath.Base(cleanTarget)
			return fmt.Errorf("sandbox: execution of %q denied by no_exec policy: %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or add %q to sandbox.<profile>.exec.allow in config.yaml", targetPath, err, bin, bin)
		}
		if len(spec.AllowedExecPaths) > 0 {
			bin := filepath.Base(cleanTarget)
			return fmt.Errorf("sandbox: execution of %q denied by allowed_exec_paths policy: %w\n  Hint: pass --sandbox-exec-allow=%s on CLI, or add %q to sandbox.<profile>.exec.allow in config.yaml", targetPath, err, bin, bin)
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
	isDenied := errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
	if !isDenied {
		lower := strings.ToLower(err.Error())
		isDenied = strings.Contains(lower, "operation not permitted") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "network is unreachable")
	}
	if !isDenied {
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
	return fmt.Errorf(
		"sandbox: outbound network socket creation denied for host %q (sandbox network: %s): %w\n  Hint: pass --sandbox-net on CLI, or set sandbox.<profile>.network: host in config.yaml",
		hostLabel, mode, err,
	)
}

// FormatEnvScrubbedDiagnostic formats a debug warning when a script queries an
// environment variable that exists on the host but was scrubbed by zero-trust
// isolation.
func FormatEnvScrubbedDiagnostic(varName string) string {
	return libkite.FormatEnvScrubbedDiagnostic(varName)
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

	if spec.Network == NetworkNone &&
		(strings.Contains(lower, "socket:") ||
			strings.Contains(lower, "network is unreachable") ||
			strings.Contains(lower, "dial tcp") ||
			strings.Contains(lower, "dial udp")) &&
		(strings.Contains(lower, "operation not permitted") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "network is unreachable")) {
		diag := FormatNetworkError(syscall.EPERM, "", spec).Error()
		if stderr == "" {
			return diag + "\n"
		}
		if !strings.HasSuffix(stderr, "\n") {
			stderr += "\n"
		}
		return stderr + diag + "\n"
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
