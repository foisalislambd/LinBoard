package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppName     = "LinBoard"
	HotkeyLabel = "Super+V"
)

// AppVersion is set at build time via -ldflags (defaults to "dev").
var AppVersion = "dev"

type Config struct {
	MaxHistory     int    `json:"max_history"`
	StartMinimized bool   `json:"start_minimized"`
	PasteOnSelect  bool   `json:"paste_on_select"` // copy + auto-paste to previous window (CopyQ-style)
	Theme          string `json:"theme"`           // light, dark, system
}

func Default() *Config {
	return &Config{
		MaxHistory:     200,
		StartMinimized: true,
		PasteOnSelect:  true,
		Theme:          "system",
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "linboard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func DataDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o700); err != nil {
		return "", err
	}
	return data, nil
}

func ImagesDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	images := filepath.Join(dir, "images")
	if err := os.MkdirAll(images, 0o700); err != nil {
		return "", err
	}
	return images, nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := Default()
		if saveErr := cfg.Save(); saveErr != nil {
			return nil, saveErr
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if c.MaxHistory < 1 {
		c.MaxHistory = Default().MaxHistory
	}
	if c.MaxHistory > 5000 {
		c.MaxHistory = 5000
	}
	switch strings.ToLower(strings.TrimSpace(c.Theme)) {
	case "light", "dark", "system":
		c.Theme = strings.ToLower(strings.TrimSpace(c.Theme))
	default:
		c.Theme = "system"
	}
}

func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}
