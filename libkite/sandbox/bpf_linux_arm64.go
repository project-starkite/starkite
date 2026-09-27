//go:build linux && arm64

package sandbox

import "golang.org/x/sys/unix"

const (
	auditArch       = unix.AUDIT_ARCH_AARCH64
	isSupportedArch = true
)

var errUnsupportedArch error = nil
