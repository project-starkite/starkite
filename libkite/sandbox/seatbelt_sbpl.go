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

// GenerateSeatbeltSBPL generates a macOS Sandbox Profile Language (SBPL)
// configuration string from an ExecutionSpec.
func GenerateSeatbeltSBPL(spec *ExecutionSpec) string {
	var b strings.Builder

	b.WriteString(";; Starkite Seatbelt Sandbox Profile\n")
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n\n")

	// Standard process capabilities and system services
	b.WriteString(";; Process execution and system capabilities\n")
	b.WriteString("(allow process-exec)\n")
	b.WriteString("(allow process-fork)\n")
	b.WriteString("(allow signal (target self))\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach-lookup)\n\n")

	// Standard host read access for system binaries, runtime libraries, and certificates
	b.WriteString(";; Host system library and binary access\n")
	b.WriteString("(allow file-read*)\n")
	b.WriteString("(allow file-read-metadata)\n\n")

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
		b.WriteString(fmt.Sprintf("(deny file-read* (subpath %q))\n", cleanHome))
		b.WriteString(fmt.Sprintf("(deny file-write* (subpath %q))\n", cleanHome))

		if !isHomeOrRoot {
			if strings.HasPrefix(cleanCwd, cleanHome) {
				b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", cleanCwd))
				b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", cleanCwd))
			}
			starkiteDir := cleanSBPLPath(filepath.Join(cleanHome, ".starkite"))
			b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", starkiteDir))
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
			if isHomeOrRoot && (cleanTarget == cleanHome || cleanTarget == "/") {
				// Under container parity, omit host $HOME or / bind mount
				continue
			}

			// If mount is inside $HOME, explicitly allow read
			if cleanHome != "" && strings.HasPrefix(cleanTarget, cleanHome) {
				b.WriteString(fmt.Sprintf("(allow file-read* (subpath %q))\n", cleanTarget))
			}

			if m.Type == MountTmpfs || m.Mode == MountRW {
				b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", cleanTarget))
				if cleanTarget == "/tmp" {
					b.WriteString("(allow file-write* (subpath \"/private/tmp\"))\n")
				}
				if cleanTarget == "/var" {
					b.WriteString("(allow file-write* (subpath \"/private/var\"))\n")
				}
				if cleanTarget == "/etc" {
					b.WriteString("(allow file-write* (subpath \"/private/etc\"))\n")
				}
			}
		}
		b.WriteString("\n")
	}

	// Target script file access: under container parity (especially if CWD == $HOME or /),
	// allow reading only the script file itself.
	if spec.ScriptFile != "" {
		cleanScript := cleanSBPLPath(spec.ScriptFile)
		b.WriteString(";; Script File Access\n")
		b.WriteString(fmt.Sprintf("(allow file-read* (literal %q))\n\n", cleanScript))
	}

	// Working directory writable if specified and not HOME or root
	if cleanCwd != "" && !isHomeOrRoot {
		b.WriteString(";; Working Directory Access\n")
		if cleanHome == "" || !strings.HasPrefix(cleanCwd, cleanHome) {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", cleanCwd))
		}
		if strings.HasPrefix(cleanCwd, "/var/") {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", "/private"+cleanCwd))
		} else if strings.HasPrefix(cleanCwd, "/private/var/") {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", strings.TrimPrefix(cleanCwd, "/private")))
		}
		if strings.HasPrefix(cleanCwd, "/tmp/") {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", "/private"+cleanCwd))
		} else if strings.HasPrefix(cleanCwd, "/private/tmp/") {
			b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", strings.TrimPrefix(cleanCwd, "/private")))
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
