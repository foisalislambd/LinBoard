package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/foisal/linboard/internal/config"
)

// logRotateBytes is the size at which the log file is rotated once.
// Tests in this package may lower it.
var logRotateBytes int64 = 2 << 20

// Init sends logs to stderr and to $XDG_STATE_HOME/linboard/linboard.log.
// Clipboard contents are never written by the logger; callers must not log them.
// The returned function closes the log file and is safe to call more than once.
func Init() (func(), error) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[LinBoard] ")
	log.SetOutput(os.Stderr)

	dir, err := config.StateDir()
	if err != nil {
		return func() {}, err
	}
	path := filepath.Join(dir, "linboard.log")
	if err := rotate(path); err != nil {
		return func() {}, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}, err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	var once sync.Once
	return func() {
		once.Do(func() { _ = f.Close() })
	}, nil
}

func rotate(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if fi.Size() < logRotateBytes {
		return nil
	}
	backup := path + ".1"
	_ = os.Remove(backup)
	return os.Rename(path, backup)
}
