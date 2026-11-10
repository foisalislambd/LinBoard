//go:build !unix

package clipboard

import (
	"os/exec"
	"time"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 200 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
