//go:build linux

package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/ipc"
)

// Acquire ensures only one LinBoard GUI instance runs. Returns a release func.
func Acquire() (func(), error) {
	dir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	runDir := filepath.Join(dir, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(runDir, "linboard.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		// Already running — forward toggle (retry while IPC server starts)
		if toggleErr := ipc.ToggleWithRetry(15, 100*time.Millisecond); toggleErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrAlreadyRunning, toggleErr)
		}
		return nil, ErrAlreadyRunning
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
