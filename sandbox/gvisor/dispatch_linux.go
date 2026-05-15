//go:build linux

package gvisor

import (
	"os"
	"path/filepath"
	"strings"

	"gvisor.dev/gvisor/runsc/cli"
	"gvisor.dev/gvisor/runsc/cmd"
	"gvisor.dev/gvisor/runsc/cmd/util"

	"github.com/project-starkite/starkite/libkite/sandbox"
)

// runscInternalGroup is the help-group name gVisor uses for boot/gofer/
// umount in its standard runsc CLI. The same string is reused here for parity.
const runscInternalGroup = "internal use only"

// DispatchSubprocess inspects argv for gVisor's self-exec personalities
// (boot, gofer, umount) plus a private "__runtime__" marker, and routes
// each to the appropriate handler. Returns true if the call was
// dispatched and the caller MUST stop further setup (cli.Run calls
// os.Exit; the __runtime__ branch returns true after stripping the
// marker so the caller knows the env was prepared).
//
// Returns false when the invocation is a normal kite CLI command and
// should fall through to cobra.
//
// This must run BEFORE cobra parses argv: cobra would reject "boot"
// as an unknown subcommand and fail the self-exec.
func DispatchSubprocess() bool {
	if isRuntimeMarker(os.Args) {
		stripRuntimeMarker()
		// Belt and suspenders: ensure inner kite knows it's sandboxed
		// even if the parent didn't set the env.
		_ = os.Setenv(sandbox.InsideEnvVar, "1")
		return false // let cobra run the real subcommand inside the sandbox
	}

	if !looksLikeRunscInvocation(os.Args) {
		return false
	}

	// looksLikeGvisorSelfExec must NOT gate this dispatch. gVisor's
	// MaybeRunAsRoot intermediate re-exec uses argv[0] = "/proc/self/exe"
	// (not "runsc-*"), so a runsc- prefix gate would incorrectly reject
	// legitimate gofer/sandbox subprocesses and break the sandbox flow.
	// Narrowing by gVisor-specific flags (--root=, --rootless) is
	// possible if the cosmetic-help leak from `kite boot --help` ever
	// becomes a real concern.

	// Register only the three "internal use only" subcommands. The
	// user-facing OCI subcommands (run, exec, kill, ...) are NOT
	// registered — gVisor's user CLI is not exposed via kite. The
	// only legitimate path into cli.Run from kite is a gVisor self-exec,
	// which only ever uses these three.
	cmds := map[util.SubCommand]string{
		new(cmd.Boot):   runscInternalGroup,
		new(cmd.Gofer):  runscInternalGroup,
		new(cmd.Umount): runscInternalGroup,
	}
	cli.Run(cmds, nil) // calls os.Exit; never returns
	return true        // unreachable
}

// looksLikeGvisorSelfExec reports whether argv[0] suggests gVisor itself
// invoked the binary (via /proc/self/exe with cmd.Args[0] rewritten to a
// "runsc-*" cosmetic name).
func looksLikeGvisorSelfExec(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return strings.HasPrefix(filepath.Base(args[0]), "runsc-")
}

// looksLikeRunscInvocation reports whether argv suggests a gVisor self-
// exec or a deliberate `kite boot|gofer|umount …` invocation.
//
// It accepts the call when EITHER:
//   - argv[0] basename starts with "runsc-" (the cosmetic process name
//     gVisor sets when it self-execs — see runsc/sandbox/sandbox.go and
//     runsc/container/container.go where Args[0] is set to
//     "runsc-sandbox" / "runsc-gofer"); OR
//   - the first non-flag positional in argv[1:] is one of "boot",
//     "gofer", or "umount". This handles direct invocation without the
//     argv[0] rewrite (e.g. `./bin/kite boot --help` for testing).
//
// Flag tokens that consume the next arg (e.g. `--root /tmp`, with the
// value as a separate token) are a known minor false-positive risk: if
// the value happens to equal one of the internal subcommand names, the
// scanner would route to gVisor. This shape isn't produced by gVisor's
// own self-exec (it always uses --flag=value form) and isn't produced
// by kite's own cobra commands, so the risk is theoretical.
func looksLikeRunscInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if strings.HasPrefix(filepath.Base(args[0]), "runsc-") {
		return true
	}
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") {
			continue
		}
		switch a {
		case "boot", "gofer", "umount":
			return true
		}
		return false
	}
	return false
}

// isRuntimeMarker reports whether argv[1] is the __runtime__ marker injected
// into the sandbox-side argv to distinguish "running normally" from
// "running inside a sandbox" without changing the script's own argv.
func isRuntimeMarker(args []string) bool {
	return len(args) >= 2 && args[1] == "__runtime__"
}

// stripRuntimeMarker removes the __runtime__ token from os.Args[1] so
// cobra sees argv as if the marker were never there. The marker lives
// at index 1 by construction (always written as the first arg of the
// inner kite invocation).
func stripRuntimeMarker() {
	if len(os.Args) < 2 || os.Args[1] != "__runtime__" {
		return
	}
	os.Args = append(os.Args[:1], os.Args[2:]...)
}
