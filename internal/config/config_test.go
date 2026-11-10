package config

import (
	"os"
	"path/filepath"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdg-data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "xdg-state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "xdg-runtime"))
	return dir
}

func TestLoadNormalizesAndPermissions(t *testing.T) {
	isolate(t)
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("{\n  \"max_history\": 0,\n  \"theme\": \"BLUE\"\n}\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxHistory != 200 || cfg.Theme != "system" {
		t.Fatalf("normalized %+v", cfg)
	}
	if !cfg.PasteOnSelect || !cfg.StartMinimized {
		t.Fatalf("missing keys should keep defaults: %+v", cfg)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("config dir mode %o", dirInfo.Mode().Perm())
	}
}

func TestCorruptConfigRecovered(t *testing.T) {
	isolate(t)
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxHistory != 200 {
		t.Fatalf("defaults not restored: %+v", cfg)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatal(err)
	}
}

func TestRelativeXDGIgnored(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	t.Setenv("XDG_DATA_HOME", "relative-data")
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, ".config", "linboard")
	if got != want {
		t.Fatalf("config dir %s, want %s", got, want)
	}
	data, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if data != filepath.Join(dir, ".local", "share", "linboard") {
		t.Fatalf("data dir %s", data)
	}
}

func TestLegacyHistoryMoved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "share"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "run"))

	legacy := filepath.Join(dir, ".config", "linboard", "data")
	if err := os.MkdirAll(filepath.Join(legacy, "images"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "clips.json"), []byte(`{"next_id":1,"clips":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "images", "a.png"), []byte("img"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "share", "linboard")
	if got != want {
		t.Fatalf("data dir %s", got)
	}
	if _, err := os.Stat(filepath.Join(want, "clips.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(want, "images", "a.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "clips.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy history still present: %v", err)
	}
}
