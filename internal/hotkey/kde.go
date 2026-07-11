package hotkey

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
	"github.com/godbus/dbus/v5"
)

type kdeBackend struct{}

func (b *kdeBackend) start(_ func()) error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	if err := setupKDEHotkey(exe); err != nil {
		return err
	}
	log.Printf("hotkey registered (KDE): %s → linboard toggle", config.HotkeyLabel)
	return nil
}

func (b *kdeBackend) stop() {}

func setupKDEHotkey(exe string) error {
	if !platform.IsKDE() {
		return skip("not KDE")
	}

	uuid := "{linboard-toggle-0001}"
	dataGroup := "Data_1 20 LinBoard"
	path := kdeHotkeysPath()

	content := readFile(path)
	if strings.Contains(content, "linboard-toggle") {
		updated, err := patchKHotkeysINI(content, dataGroup, exe+" toggle")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return err
		}
		reloadKHotkeys()
		return nil
	}

	lines := []string{
		"",
		"[" + dataGroup + "]",
		"Comment=LinBoard clipboard history",
		"Enabled=true",
		"Name=LinBoard",
		"Type=SIMPLE_COMMAND_DATA",
		"Command=" + exe + " toggle",
		"Uuid=" + uuid,
		"",
		"[" + dataGroup + "|0 Trigger]",
		"Comment=LinBoard trigger",
		"Enabled=true",
		"Type=SHORTCUT_TRIGGER_DATA",
		"Uuid=" + uuid + "-trigger",
		"Key=Meta+V",
		"",
		"[" + dataGroup + "|0 Action/0]",
		"Type=COMMAND_URLS",
		"Uuid=" + uuid + "-action",
		"Command=" + exe + " toggle",
	}
	if err := appendFile(path, strings.Join(lines, "\n")+"\n"); err != nil {
		return err
	}
	reloadKHotkeys()
	return nil
}

func kdeHotkeysPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "khotkeysrc")
}

func patchKHotkeysINI(content, group, command string) (string, error) {
	section := "[" + group + "]"
	if !strings.Contains(content, section) {
		return "", fmt.Errorf("khotkeys section %q not found", group)
	}
	// Update every Command= line that runs linboard toggle (main + action sections).
	re := regexp.MustCompile(`(?m)^Command=.*toggle\s*$`)
	if !re.MatchString(content) {
		return "", fmt.Errorf("khotkeys: no linboard toggle command found")
	}
	return re.ReplaceAllString(content, "Command="+command), nil
}

func reloadKHotkeys() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer conn.Close()
	for _, spec := range []struct {
		bus    string
		iface  string
	}{
		{"org.kde.kded6", "org.kde.kded6"},
		{"org.kde.kded5", "org.kde.kded5"},
	} {
		obj := conn.Object(spec.bus, "/kded")
		_ = obj.Call(spec.iface+".reloadModule", 0, "khotkeys").Err
	}
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
