package hotkey

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/foisal/linboard/internal/config"
)

// RegisterSystemShortcut binds Super+V to `linboard toggle` using the desktop environment API.
func RegisterSystemShortcut() error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	return RegisterSystemShortcutAt(exe)
}

// RegisterSystemShortcutAt binds Super+V using a specific binary path.
func RegisterSystemShortcutAt(exe string) error {
	return SetupAt(exe)
}

// ExecutableForShortcut returns the binary used for Super+V (prefer ~/.local/bin).
func ExecutableForShortcut() (string, error) {
	if local := filepath.Join(os.Getenv("HOME"), ".local", "bin", "linboard"); fileExecutable(local) {
		return local, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func fileExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

func registerSystemShortcut(exe string) error {
	backends := []struct {
		name string
		fn   func(string) error
	}{
		{"GNOME", setupGNOME},
		{"KDE", setupKDEHotkey},
		{"XFCE", setupXFCEHotkey},
		{"Cinnamon", setupCinnamonHotkey},
		{"MATE", setupMATEHotkey},
	}
	var errs []string
	for _, b := range backends {
		if err := b.fn(exe); err == nil {
			return nil
		} else if !isSkipErr(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", b.name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("shortcut registration failed: %s", strings.Join(errs, "; "))
	}
	return fmt.Errorf("no supported desktop environment for automatic %s binding", config.HotkeyLabel)
}

func isSkipErr(err error) bool {
	return strings.Contains(err.Error(), "skip:")
}

func skip(format string, args ...any) error {
	return fmt.Errorf("skip: "+format, args...)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %v: %w (%s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func hasBin(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}
