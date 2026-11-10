//go:build linux

package singleinstance

import (
	"errors"
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
	dir, err := config.RuntimeDir()
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, "linboard.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, err
		}
		// Already running — forward toggle (retry while IPC server starts)
		if toggleErr := ipc.ToggleWithRetry(15, 100*time.Millisecond); toggleErr != nil {
			return nil, fmt.Errorf("LinBoard is already running, but the history window could not be opened: %w", toggleErr)
		}
		return nil, ErrAlreadyRunning
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
