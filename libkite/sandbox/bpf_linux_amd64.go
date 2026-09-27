//go:build linux && amd64

package sandbox

import "golang.org/x/sys/unix"

const (
	auditArch       = unix.AUDIT_ARCH_X86_64
	isSupportedArch = true
)

var errUnsupportedArch error = nil
