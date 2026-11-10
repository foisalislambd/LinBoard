package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesStateFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	prevOut := log.Writer()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})

	closeLog, err := Init()
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("hello-log-line")
	closeLog()

	path := filepath.Join(dir, "state", "linboard", "linboard.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "hello-log-line") {
		t.Fatalf("log %q", b)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", info.Mode().Perm())
	}
}
