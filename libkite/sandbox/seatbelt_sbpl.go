package sandbox

import (
	"fmt"
	"strings"
)

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

			if m.Type == MountTmpfs || m.Mode == MountRW {
				b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n", target))
			}
		}
		b.WriteString("\n")
	}

	// Working directory writable if specified
	if spec.Cwd != "" {
		b.WriteString(";; Working Directory Access\n")
		b.WriteString(fmt.Sprintf("(allow file-write* (subpath %q))\n\n", spec.Cwd))
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
