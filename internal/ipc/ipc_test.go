package ipc

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "run"))
}

func TestToggleAndSocketMode(t *testing.T) {
	isolate(t)
	var n atomic.Int32
	srv, err := StartServer(func() { n.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)

	path, err := SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %o", info.Mode().Perm())
	}
	if err := Toggle(); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 1 {
		t.Fatalf("toggles %d", n.Load())
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("nope\n")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	nr, err := conn.Read(buf)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:nr]) != "err\n" {
		t.Fatalf("reply %q", buf[:nr])
	}
	if n.Load() != 1 {
		t.Fatal("unknown command toggled")
	}

	srv.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket left behind: %v", err)
	}
}
