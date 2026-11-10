//go:build linux

package singleinstance

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/foisal/linboard/internal/ipc"
)

func TestSecondInstanceToggles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "run"))

	release, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	_, err = Acquire()
	if err == nil || errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected toggle failure without a server, got %v", err)
	}

	srv, err := ipc.StartServer(func() {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)

	_, err = Acquire()
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("got %v", err)
	}
}
