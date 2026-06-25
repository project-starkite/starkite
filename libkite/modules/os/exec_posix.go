//go:build !windows

package osmod

import (
	"os/exec"
	"syscall"
)

const supportsUserSwitch = true

func configureCredential(cmd *exec.Cmd, uid, gid uint32, hasUID, hasGID bool) {
	if !hasUID && !hasGID {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cred := &syscall.Credential{}
	if hasUID {
		cred.Uid = uid
	}
	if hasGID {
		cred.Gid = gid
	}
	cmd.SysProcAttr.Credential = cred
}
