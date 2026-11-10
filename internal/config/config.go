package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/foisal/linboard/internal/atomicfile"
)

const (
	AppName     = "LinBoard"
	HotkeyLabel = "Super+V"
)

// AppVersion is the VERSION.md value, injected with -ldflags by make, CI, and
// the release workflow. A plain `go build` leaves it as "dev".
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

// ConfigHome is $XDG_CONFIG_HOME or ~/.config.
// Relative XDG paths are ignored so a crafted environment cannot redirect
// writes onto the process working directory.
func ConfigHome() (string, error) {
	return xdgHome("XDG_CONFIG_HOME", ".config")
}

// DataHome is $XDG_DATA_HOME or ~/.local/share.
func DataHome() (string, error) {
	return xdgHome("XDG_DATA_HOME", filepath.Join(".local", "share"))
}

// StateHome is $XDG_STATE_HOME or ~/.local/state.
func StateHome() (string, error) {
	return xdgHome("XDG_STATE_HOME", filepath.Join(".local", "state"))
}

func xdgHome(envKey, fallbackRel string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" && filepath.IsAbs(v) {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallbackRel), nil
}

func ConfigDir() (string, error) {
	base, err := ConfigHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "linboard")
	if err := mkdirPrivate(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// DataDir is the history directory ($XDG_DATA_HOME/linboard).
// A history written by older releases under ~/.config/linboard/data is moved here.
func DataDir() (string, error) {
	base, err := DataHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "linboard")
	if err := migrateLegacyData(dir); err != nil {
		return "", err
	}
	if err := mkdirPrivate(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// RuntimeDir is $XDG_RUNTIME_DIR/linboard, or DataDir/run when the runtime dir is unset.
func RuntimeDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); v != "" && filepath.IsAbs(v) {
		dir := filepath.Join(v, "linboard")
		if err := mkdirPrivate(dir); err != nil {
			return "", err
		}
		return dir, nil
	}
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "run")
	if err := mkdirPrivate(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// StateDir holds logs ($XDG_STATE_HOME/linboard).
func StateDir() (string, error) {
	base, err := StateHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "linboard")
	if err := mkdirPrivate(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func ImagesDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	images := filepath.Join(dir, "images")
	if err := mkdirPrivate(images); err != nil {
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

// AutostartDir is the XDG autostart directory.
func AutostartDir() (string, error) {
	base, err := ConfigHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "autostart")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ApplicationsDir is $XDG_DATA_HOME/applications.
func ApplicationsDir() (string, error) {
	base, err := DataHome()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
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
		bak := path + ".corrupt"
		_ = os.Remove(bak)
		if renErr := os.Rename(path, bak); renErr != nil {
			return nil, err
		}
		log.Printf("config.json is unreadable — moved aside to %s (%v)", bak, err)
		cfg = Default()
		if saveErr := cfg.Save(); saveErr != nil {
			return nil, saveErr
		}
		return cfg, nil
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
	data = append(data, '\n')
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func mkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}
