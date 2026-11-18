package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/project-starkite/starkite/libkite"
)

func cleanSBPLPath(p string) string {
	if p == "" {
		return ""
	}
	sanitized := strings.NewReplacer("\x00", "", "\r", "", "\n", "", "\"", "").Replace(p)
	if sanitized == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(sanitized))
}

func hasSBPLPathPrefix(path, prefix string) bool {
	if path == "" || prefix == "" {
		return false
	}
	if prefix == "/" {
		return strings.HasPrefix(path, "/")
	}
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func isAllowedNativeTmpfsDestination(dst string) bool {
	if dst == "" {
		return false
	}
	allowedRoots := []string{
		"/tmp",
		"/private/tmp",
		"/var/tmp",
		"/private/var/tmp",
		"/var/folders",
		"/private/var/folders",
	}
	if sysTmp := cleanSBPLPath(os.TempDir()); sysTmp != "" && sysTmp != "/" {
		allowedRoots = append(allowedRoots, sysTmp)
	}
	for _, variant := range sbplPathVariants(dst) {
		for _, root := range allowedRoots {
			if hasSBPLPathPrefix(variant, root) {
				return true
			}
		}
	}
	return false
}

func sbplPathVariants(p string) []string {
	clean := cleanSBPLPath(p)
	if clean == "" {
		return nil
	}
	seen := map[string]bool{clean: true}
	out := []string{clean}
	add := func(cand string) {
		c := cleanSBPLPath(cand)
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}

	for _, prefix := range []string{"/tmp", "/var", "/etc"} {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			add("/private" + clean)
		}
		privPrefix := "/private" + prefix
		if clean == privPrefix || strings.HasPrefix(clean, privPrefix+"/") {
			add(strings.TrimPrefix(clean, "/private"))
		}
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		add(resolved)
	}
	return out
}

// GenerateSeatbeltSBPL generates a macOS Sandbox Profile Language (SBPL)
// configuration string from an ExecutionSpec.
func GenerateSeatbeltSBPL(spec *ExecutionSpec) string {
	var b strings.Builder

	b.WriteString(";; Starkite Seatbelt Sandbox Profile\n")
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n\n")

	// Standard process capabilities and system services
	b.WriteString(";; Process execution and system capabilities\n")
	if spec.NoExec || len(spec.AllowedExecPaths) > 0 {
		b.WriteString("(deny process-exec)\n")
	} else {
		b.WriteString("(allow process-exec)\n")
	}
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow signal (target self))\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n\n")

	// Standard host read access for system binaries, runtime libraries, and certificates
	b.WriteString(";; Host system library and binary access\n")
	b.WriteString("(allow file-read*)\n")
	b.WriteString("(allow file-read-metadata)\n\n")

	// Shared memory (/dev/shm and /run/shm) isolation
	b.WriteString(";; Shared memory (/dev/shm) isolation\n")
	b.WriteString("(deny file-read* (subpath \"/dev/shm\"))\n")
	b.WriteString("(deny file-write* (subpath \"/dev/shm\"))\n")
	b.WriteString("(deny file-read* (subpath \"/run/shm\"))\n")
	b.WriteString("(deny file-write* (subpath \"/run/shm\"))\n\n")

	// Standard terminal and device handles
	b.WriteString(";; Standard terminal, null/zero devices, and stdout/stderr pipes\n")
	b.WriteString("(allow file-write-data\n")
	b.WriteString("    (literal \"/dev/tty\")\n")
	b.WriteString("    (literal \"/dev/null\")\n")
	b.WriteString("    (literal \"/dev/zero\")\n")
	b.WriteString("    (literal \"/dev/dtracehelper\")\n")
	b.WriteString(")\n")
	b.WriteString("(allow file-write*\n")
	b.WriteString("    (literal \"/dev/null\")\n")
	b.WriteString("    (literal \"/dev/zero\")\n")
	b.WriteString(")\n\n")

	var isHomeOrRoot bool
	cleanCwd := ""
	cleanHome := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		cleanHome = cleanSBPLPath(home)
	}
	homeVariants := sbplPathVariants(cleanHome)
	var cwdVariants []string
	if spec.Cwd != "" {
		cleanCwd = cleanSBPLPath(spec.Cwd)
		cwdVariants = sbplPathVariants(cleanCwd)
		for _, cv := range cwdVariants {
			if cv == "/" {
				isHomeOrRoot = true
				break
			}
			if slices.Contains(homeVariants, cv) {
				isHomeOrRoot = true
			}
			if isHomeOrRoot {
				break
			}
		}
	}

	// Container-parity isolation for home directory:
	// Deny broad access to $HOME so sensitive files (~/.ssh, ~/.aws, etc.) are never exposed.
	if cleanHome != "" {
		b.WriteString(";; Container-parity isolation for home directory\n")
		for _, hVar := range homeVariants {
			b.WriteString(fmt.Sprintf("(deny file-read* (subpath %q))\n", hVar))
			b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", hVar))
		}

		if !isHomeOrRoot {
			starkiteDir := cleanSBPLPath(filepath.Join(cleanHome, ".starkite"))
			for _, stVar := range sbplPathVariants(starkiteDir) {
				b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", stVar))
			}
		}
		b.WriteString("\n")
	}

	cwdMounted := false
	for _, m := range spec.Mounts {
		target := m.Source
		if target == "" {
			target = m.Destination
		}
		if cleanSBPLPath(target) == cleanCwd {
			cwdMounted = true
			break
		}
	}

	// Working directory isolation: if CWD is not mounted, deny reading and writing
	// unless already isolated by $HOME.
	if !cwdMounted && cleanCwd != "" && !isHomeOrRoot && cleanCwd != "/tmp" && cleanCwd != "/private/tmp" {
		cwdInsideHome := false
		for _, cv := range cwdVariants {
			for _, hv := range homeVariants {
				if hasSBPLPathPrefix(cv, hv) {
					cwdInsideHome = true
					break
				}
			}
			if cwdInsideHome {
				break
			}
		}
		if !cwdInsideHome {
			b.WriteString(";; Working directory isolation (unmounted)\n")
			for _, cwdVar := range cwdVariants {
				b.WriteString(fmt.Sprintf("(deny file-read* (subpath %q))\n", cwdVar))
				b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", cwdVar))
			}
			b.WriteString("\n")
		}
	}

	// Explicit writable mounts from ExecutionSpec
	if len(spec.Mounts) > 0 {
		b.WriteString(";; Spec Writable Mount Rules\n")
		for _, m := range spec.Mounts {
			target := m.Source
			if target == "" {
				target = m.Destination
			}
			if target == "" {
				continue
			}

			cleanTarget := cleanSBPLPath(target)
			cleanDst := cleanSBPLPath(m.Destination)
			if isHomeOrRoot && (slices.Contains(homeVariants, cleanTarget) || cleanTarget == "/") {
				// Under container parity, omit host $HOME or / bind mount
				continue
			}
			if cleanTarget == "/dev/shm" || strings.HasPrefix(cleanTarget, "/dev/shm/") ||
				cleanTarget == "/run/shm" || strings.HasPrefix(cleanTarget, "/run/shm/") ||
				cleanDst == "/dev/shm" || strings.HasPrefix(cleanDst, "/dev/shm/") ||
				cleanDst == "/run/shm" || strings.HasPrefix(cleanDst, "/run/shm/") {
				continue
			}

			// Explicitly allow read access for any mount (ensures subpaths under denied $HOME or CWD can be read)
			for _, variant := range sbplPathVariants(cleanTarget) {
				b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", variant))
			}

			if m.Type == MountTmpfs || m.Mode == MountRW {
				for _, variant := range sbplPathVariants(cleanTarget) {
					b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", variant))
					if !spec.NoExec {
						b.WriteString(fmt.Sprintf("(deny process-exec (subpath %q))\n", variant))
					}
				}
			}
		}
		b.WriteString("\n")
	}

	// Write-XOR-Execute (W^X) baseline on /tmp and /private/tmp
	if !spec.NoExec {
		b.WriteString(";; Write-XOR-Execute (W^X) on scratch directories\n")
		b.WriteString("(deny process-exec (subpath \"/tmp\"))\n")
		b.WriteString("(deny process-exec (subpath \"/private/tmp\"))\n\n")
	}

	// Target script file access: under container parity (especially if CWD == $HOME or /),
	// allow reading only the script file itself.
	if spec.ScriptFile != "" {
		cleanScript := cleanSBPLPath(spec.ScriptFile)
		b.WriteString(";; Script File Access\n")
		for _, sVar := range sbplPathVariants(cleanScript) {
			b.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n", sVar))
		}
		b.WriteString("\n")
	}

	// Explicit executable allowlist when AllowedExecPaths is specified and NoExec is false
	if !spec.NoExec && len(spec.AllowedExecPaths) > 0 {
		b.WriteString(";; Allowed Executable Paths\n")
		for _, p := range spec.AllowedExecPaths {
			cleanP := cleanSBPLPath(p)
			if cleanP == "" {
				continue
			}
			for _, v := range sbplPathVariants(cleanP) {
				b.WriteString(fmt.Sprintf("(allow process-exec (literal %q))\n", v))
				b.WriteString(fmt.Sprintf("(allow process-exec (subpath %q))\n", v))
				for _, hv := range homeVariants {
					if hasSBPLPathPrefix(v, hv) {
						b.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n", v))
						break
					}
				}
				for _, cv := range cwdVariants {
					if hasSBPLPathPrefix(v, cv) {
						b.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n", v))
						break
					}
				}
			}
			if resolved, evalErr := filepath.EvalSymlinks(cleanP); evalErr == nil {
				cleanResolved := cleanSBPLPath(resolved)
				if cleanResolved != "" && cleanResolved != cleanP {
					for _, v := range sbplPathVariants(cleanResolved) {
						b.WriteString(fmt.Sprintf("(allow process-exec (literal %q))\n", v))
						b.WriteString(fmt.Sprintf("(allow process-exec (subpath %q))\n", v))
						for _, hv := range homeVariants {
							if hasSBPLPathPrefix(v, hv) {
								b.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n", v))
								break
							}
						}
						for _, cv := range cwdVariants {
							if hasSBPLPathPrefix(v, cv) {
								b.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n", v))
								break
							}
						}
					}
				}
			}
		}
		b.WriteString("\n")
	}

	// Network access rules
	b.WriteString(";; Network Access Rules\n")
	switch spec.Network {
	case NetworkHost:
		if len(spec.NetworkAllow) == 0 || slices.Contains(spec.NetworkAllow, "*") {
			b.WriteString("(allow network*)\n")
			b.WriteString("(allow system-socket)\n")
		} else {
			b.WriteString("(allow system-socket)\n")
			b.WriteString("(allow network-outbound (literal \"/private/var/run/mDNSResponder\"))\n")
			b.WriteString("(allow network* (local ip \"localhost:*\"))\n")
			b.WriteString("(allow network* (remote ip \"localhost:*\"))\n")
			for _, ep := range spec.NetworkAllow {
				_, port, hasPort := libkite.SplitEndpointHostPort(ep)
				if hasPort && port != "" && port != "*" {
					portNum, err := strconv.Atoi(port)
					if err != nil || portNum < 1 || portNum > 65535 {
						continue
					}
					b.WriteString(fmt.Sprintf("(allow network-outbound (remote tcp \"*:%d\"))\n", portNum))
				} else {
					b.WriteString("(allow network-outbound (remote tcp \"*:443\"))\n")
					b.WriteString("(allow network-outbound (remote tcp \"*:80\"))\n")
				}
			}
		}
	case NetworkLoopback:
		b.WriteString("(allow network* (local ip \"localhost:*\"))\n")
		b.WriteString("(allow network* (remote ip \"localhost:*\"))\n")
		b.WriteString("(allow network-outbound (to unix-socket))\n")
		b.WriteString("(allow network-inbound (to unix-socket))\n")
	case NetworkNone, "":
		b.WriteString("(deny network*)\n")
	}

	return b.String()
}
