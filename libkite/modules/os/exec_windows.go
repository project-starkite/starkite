//go:build windows

package osmod

import (
	"os/exec"
)

const supportsUserSwitch = false

func configureCredential(cmd *exec.Cmd, uid, gid uint32, hasUID, hasGID bool) {
	// No-op stub for compilation on Windows.
	// runCmd will return an error before this is called if user/group switching is requested.
}
