//go:build linux && !amd64 && !arm64

package sandbox

import "errors"

const (
	auditArch       = 0
	isSupportedArch = false
)

var errUnsupportedArch = errors.New("sandbox: seccomp-bpf is not supported on this CPU architecture")
