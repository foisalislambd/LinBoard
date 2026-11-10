//go:build unix

package clipboard

import (
	"os/exec"
	"syscall"
	"time"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 200 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Kill the group so a forked clipboard helper cannot outlive the parent
		// and keep the selection (or this process) busy after shutdown.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
}
