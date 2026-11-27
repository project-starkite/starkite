//go:build !unix

package sandbox

import (
	"os/exec"
	"time"
)

const subprocessWaitDelay = 2 * time.Second

func configureSubprocessCancel(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.WaitDelay = subprocessWaitDelay
}

func reapSubprocessGroup(cmd *exec.Cmd) {}
