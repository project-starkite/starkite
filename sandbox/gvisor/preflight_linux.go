//go:build linux

package gvisor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Kernel sysctl paths gVisor needs in friendly state.
//
// The two paths below are exposed as variables (not constants) only so
// preflight tests can redirect them to a fake /proc directory.
var (
	procApparmorRestrictUserns = "/proc/sys/kernel/apparmor_restrict_unprivileged_userns"
	procUnprivilegedUserns     = "/proc/sys/kernel/unprivileged_userns_clone"
)

// preflight checks the host kernel state gVisor's rootless mode requires.
// Returns a friendly, actionable error when a known-blocking sysctl is set
// to a value that would later surface as a cryptic gVisor failure deep
// inside container.Run.
//
// Each check is best-effort: if the sysctl file doesn't exist (older
// kernels, non-Ubuntu distros), it's treated as "no restriction" and
// preflight passes. Read errors are similarly non-fatal — let the real
// gVisor path surface whatever's actually wrong.
func preflight() error {
	if err := checkApparmorUserns(); err != nil {
		return err
	}
	if err := checkUnprivilegedUserns(); err != nil {
		return err
	}
	return nil
}

// checkApparmorUserns guards against Ubuntu 23.10+'s default
// kernel.apparmor_restrict_unprivileged_userns=1, which silently kills
// gVisor's user-namespace fork/exec with "permission denied". The
// sysctl is Ubuntu-specific; on other distros the file is absent and
// the check passes silently.
func checkApparmorUserns() error {
	v, err := readSysctl(procApparmorRestrictUserns)
	if err != nil {
		return nil // ENOENT or unreadable — not our concern
	}
	if v != "1" {
		return nil
	}
	return fmt.Errorf(
		"sandbox unavailable: kernel.apparmor_restrict_unprivileged_userns=1 " +
			"(Ubuntu 23.10+ default) blocks gVisor's user-namespace setup. " +
			"Either install kite's AppArmor profile (see docs/guides/sandbox.md) " +
			"or temporarily disable the restriction with " +
			"`sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` " +
			"(reverts on reboot).")
}

// checkUnprivilegedUserns guards against
// kernel.unprivileged_userns_clone=0, which disables unprivileged user
// namespaces entirely. Default is 1 on every modern Linux distro; only
// hardened/custom kernels set it to 0.
func checkUnprivilegedUserns() error {
	v, err := readSysctl(procUnprivilegedUserns)
	if err != nil {
		return nil
	}
	if v != "0" {
		return nil
	}
	return fmt.Errorf(
		"sandbox unavailable: kernel.unprivileged_userns_clone=0 disables " +
			"unprivileged user namespaces, which gVisor's rootless mode " +
			"requires. Enable with `sudo sysctl -w kernel.unprivileged_userns_clone=1`.")
}

// readSysctl reads a /proc/sys file and returns the trimmed string value.
// Returns fs.ErrNotExist for missing files so callers can distinguish
// "file not present" from other read errors.
func readSysctl(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fs.ErrNotExist
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
