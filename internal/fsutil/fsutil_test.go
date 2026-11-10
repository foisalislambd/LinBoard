package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithinDir(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "images", "a.png")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !WithinDir(dir, inside) {
		t.Fatal("expected inside")
	}
	if WithinDir(dir, dir) {
		t.Fatal("directory itself is not inside")
	}
	outside := filepath.Join(filepath.Dir(dir), "other")
	if WithinDir(dir, outside) {
		t.Fatal("expected outside")
	}
	if WithinDir(dir, "relative.png") {
		t.Fatal("relative path should be rejected")
	}
}

func TestReadRegularFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegularFile(link, 100); err == nil {
		t.Fatal("symlink should be refused")
	}
	got, err := ReadRegularFile(target, 100)
	if err != nil || string(got) != "secret" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := ReadRegularFile(target, 3); err == nil {
		t.Fatal("oversize file should be refused")
	}
}
