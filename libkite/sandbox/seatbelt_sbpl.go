package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cleanSBPLPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(p))
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
	if spec.Cwd != "" {
		cleanCwd = cleanSBPLPath(spec.Cwd)
		if cleanCwd == "/" || (cleanHome != "" && cleanCwd == cleanHome) {
			isHomeOrRoot = true
		}
	}

	// Container-parity isolation for home directory:
	// Deny broad access to $HOME so sensitive files (~/.ssh, ~/.aws, etc.) are never exposed.
	if cleanHome != "" {
		b.WriteString(";; Container-parity isolation for home directory\n")
		for _, hVar := range sbplPathVariants(cleanHome) {
			b.WriteString(fmt.Sprintf("(deny file-read* (subpath %q))\n", hVar))
			b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", hVar))
		}

		if !isHomeOrRoot {
			if strings.HasPrefix(cleanCwd, cleanHome) {
				for _, cwdVar := range sbplPathVariants(cleanCwd) {
					b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", cwdVar))
					b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", cwdVar))
				}
			}
			starkiteDir := cleanSBPLPath(filepath.Join(cleanHome, ".starkite"))
			for _, stVar := range sbplPathVariants(starkiteDir) {
				b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", stVar))
			}
		}
		b.WriteString("\n")
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
			if isHomeOrRoot && (cleanTarget == cleanHome || cleanTarget == "/") {
				// Under container parity, omit host $HOME or / bind mount
				continue
			}
			if cleanTarget == "/dev/shm" || strings.HasPrefix(cleanTarget, "/dev/shm/") ||
				cleanTarget == "/run/shm" || strings.HasPrefix(cleanTarget, "/run/shm/") ||
				cleanDst == "/dev/shm" || strings.HasPrefix(cleanDst, "/dev/shm/") ||
				cleanDst == "/run/shm" || strings.HasPrefix(cleanDst, "/run/shm/") {
				continue
			}

			// If mount is inside $HOME, explicitly allow read
			if cleanHome != "" && strings.HasPrefix(cleanTarget, cleanHome) {
				for _, variant := range sbplPathVariants(cleanTarget) {
					b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", variant))
				}
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

	// Working directory writable if specified and not HOME or root
	if cleanCwd != "" && !isHomeOrRoot {
		b.WriteString(";; Working Directory Access\n")
		if cleanHome == "" || !strings.HasPrefix(cleanCwd, cleanHome) {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", cleanCwd))
		}
		if !spec.NoExec {
			b.WriteString(fmt.Sprintf("(deny process-exec (subpath %q))\n", cleanCwd))
		}
		if strings.HasPrefix(cleanCwd, "/var/") {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", "/private"+cleanCwd))
			if !spec.NoExec {
				b.WriteString(fmt.Sprintf("(deny process-exec (subpath %q))\n", "/private"+cleanCwd))
			}
		} else if strings.HasPrefix(cleanCwd, "/private/var/") {
			trimmed := strings.TrimPrefix(cleanCwd, "/private")
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", trimmed))
			if !spec.NoExec {
				b.WriteString(fmt.Sprintf("(deny process-exec (subpath %q))\n", trimmed))
			}
		}
		if strings.HasPrefix(cleanCwd, "/tmp/") {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", "/private"+cleanCwd))
			if !spec.NoExec {
				b.WriteString(fmt.Sprintf("(deny process-exec (subpath %q))\n", "/private"+cleanCwd))
			}
		} else if strings.HasPrefix(cleanCwd, "/private/tmp/") {
			trimmed := strings.TrimPrefix(cleanCwd, "/private")
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", trimmed))
			if !spec.NoExec {
				b.WriteString(fmt.Sprintf("(deny process-exec (subpath %q))\n", trimmed))
			}
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
			b.WriteString(fmt.Sprintf("(allow process-exec (subpath %q))\n", cleanP))
			if resolved, evalErr := filepath.EvalSymlinks(cleanP); evalErr == nil {
				cleanResolved := cleanSBPLPath(resolved)
				if cleanResolved != "" && cleanResolved != cleanP {
					b.WriteString(fmt.Sprintf("(allow process-exec (subpath %q))\n", cleanResolved))
				}
			}
		}
		b.WriteString("\n")
	}

	// Network access rules
	b.WriteString(";; Network Access Rules\n")
	switch spec.Network {
	case NetworkHost:
		b.WriteString("(allow network*)\n")
		b.WriteString("(allow system-socket)\n")
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
