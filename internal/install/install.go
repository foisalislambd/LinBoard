package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/foisal/linboard/internal/assets"
	"github.com/foisal/linboard/internal/clipboard"
	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/hotkey"
)

// Run installs LinBoard binary, desktop file, autostart entry, and Super+V shortcut.
func Run() error {
	exe, err := hotkeyExecutable()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(binDir, "linboard")
	if exe != dest {
		if err := copyFile(exe, dest); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
	}
	launcher := filepath.Join(binDir, "linboard-start")
	if err := writeLauncher(launcher, dest); err != nil {
		return fmt.Errorf("install launcher: %w", err)
	}

	desktopDir, err := config.ApplicationsDir()
	if err != nil {
		return err
	}
	desktopBody := desktopEntry(launcher, false)
	if err := os.WriteFile(filepath.Join(desktopDir, "linboard.desktop"), []byte(desktopBody), 0o644); err != nil {
		return err
	}

	autostartDir, err := config.AutostartDir()
	if err != nil {
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
%s`, desktopExec(execPath), extra)
}

// desktopExec quotes a path per the desktop entry spec when it contains
// characters that the Exec key would otherwise split or expand.
func desktopExec(path string) string {
	if !strings.ContainsAny(path, " \t\"\\$`") {
		return path
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range path {
		switch r {
		case '\\', '"', '`', '$':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	// Write to a temp file and rename so a running binary is not truncated
	// in place (that returns ETXTBSY on Linux).
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, in, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
if getent group input 2>/dev/null | awk -F: '{print $4}' | tr ',' '\n' | grep -Fxq -e "$USER" \
   && ! id -nG | tr ' ' '\n' | grep -Fxq input; then
  exec sg input -c "$(printf 'exec %%q ' "$LB" "$@")"
fi
run "$@"
`, linboardExe)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return err
	}
	return nil
}
