package install

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/foisal/linboard/internal/assets"
	"github.com/foisal/linboard/internal/clipboard"
	"github.com/foisal/linboard/internal/hotkey"
)

// Run installs LinBoard binary, desktop file, autostart entry, and Super+V shortcut.
func Run() error {
	exe, err := hotkeyExecutable()
	if err != nil {
		return err
	}

	binDir := filepath.Join(os.Getenv("HOME"), ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(binDir, "linboard")
	if exe != dest {
		if err := copyFile(exe, dest); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
		_ = os.Chmod(dest, 0o755)
	}
	launcher := filepath.Join(binDir, "linboard-start")
	if err := writeLauncher(launcher, dest); err != nil {
		return fmt.Errorf("install launcher: %w", err)
	}

	share := filepath.Join(os.Getenv("HOME"), ".local", "share")
	desktopDir := filepath.Join(share, "applications")
	if err := os.MkdirAll(desktopDir, 0o755); err != nil {
		return err
	}
	desktopBody := desktopEntry(launcher, false)
	if err := os.WriteFile(filepath.Join(desktopDir, "linboard.desktop"), []byte(desktopBody), 0o644); err != nil {
		return err
	}

	autostartDir := filepath.Join(os.Getenv("HOME"), ".config", "autostart")
	if err := os.MkdirAll(autostartDir, 0o755); err != nil {
		return err
	}
	autostartBody := desktopEntry(launcher, true)
	if err := os.WriteFile(filepath.Join(autostartDir, "linboard.desktop"), []byte(autostartBody), 0o644); err != nil {
		return err
	}

	if err := assets.InstallThemeIcons(); err != nil {
		return fmt.Errorf("install icons: %w", err)
	}

	if err := hotkey.SetupAt(dest); err != nil {
		return fmt.Errorf("shortcut setup failed: %w", err)
	}

	report := hotkey.Verify(dest)
	hotkey.PrintVerify(report)
	clipboard.PrintPasteSetup()

	fmt.Printf("Installed: %s\n", dest)
	fmt.Println("LinBoard starts automatically on login.")
	if !report.Healthy() {
		return fmt.Errorf("install finished with shortcut issues — run: linboard doctor")
	}
	return nil
}

func hotkeyExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func desktopEntry(execPath string, autostart bool) string {
	extra := ""
	if autostart {
		extra = "X-GNOME-Autostart-enabled=true\n"
	}
	return fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=LinBoard
GenericName=Clipboard Manager
Comment=Windows-style clipboard history for Linux
Exec=%s
Icon=linboard
Terminal=false
Categories=Utility;
StartupNotify=false
%s`, execPath, extra)
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o755)
}

func writeLauncher(path, linboardExe string) error {
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

# Detach when run from a terminal so the app survives after the shell closes.
if [[ -t 1 ]] && [[ "${LINBOARD_FOREGROUND:-}" != "1" ]]; then
  LINBOARD_FOREGROUND=1 nohup "$0" "$@" >/dev/null 2>&1 &
  disown 2>/dev/null || true
  exit 0
fi

LB=%q
run() {
  exec "$LB" "$@"
}
if [[ -w /dev/uinput ]]; then
  run "$@"
fi
if getent group input 2>/dev/null | awk -F: '{print $4}' | tr ',' '\n' | grep -qx "$USER" \
   && ! id -nG | tr ' ' '\n' | grep -qx input; then
  exec sg input -c "$(printf 'exec %%q ' "$LB" "$@")"
fi
run "$@"
`, linboardExe)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return err
	}
	return nil
}

