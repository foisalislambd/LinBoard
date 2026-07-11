package hotkey

import (
	"log"
	"strings"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

const (
	gnomeShellKeybindings = "org.gnome.shell.keybindings"
	gnomeMessageTrayKey     = "toggle-message-tray"
)

const gnomeBindingName = "custom-linboard"

type gnomeBackend struct{}

func (b *gnomeBackend) start(_ func()) error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	if err := setupGNOME(exe); err != nil {
		return err
	}
	wrapper, _ := ToggleWrapperPath()
	log.Printf("hotkey ready (GNOME): %s → %s", config.HotkeyLabel, wrapper)
	return nil
}

func (b *gnomeBackend) stop() {}

func gnomeBindingPath() string {
	return "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/" + gnomeBindingName + "/"
}

func gnomeBindingSchema() string {
	return gnomeMediaKeys + ".custom-keybinding:" + gnomeBindingPath()
}

func setupGNOME(exe string) error {
	if !platform.IsGNOME() {
		return skip("not GNOME")
	}
	if err := ensureGNOMEMediaKeys(); err != nil && !isSkipErr(err) {
		log.Printf("hotkey: %v", err)
	}
	if err := releaseGNOMESuperVConflict(); err != nil {
		return err
	}
	wrapper, err := EnsureToggleWrapper(exe)
	if err != nil {
		return err
	}
	return applyGNOMEHotkey(wrapper)
}

func applyGNOMEHotkey(command string) error {
	schema := gnomeBindingSchema()
	bindingPath := gnomeBindingPath()

	paths, err := gsettingsListPaths()
	if err != nil {
		return err
	}
	if !containsPath(paths, bindingPath) {
		paths = append(paths, bindingPath)
		if err := gsettingsSetPaths(paths); err != nil {
			return err
		}
	}

	if err := gsettingsSet(schema, "name", "LinBoard"); err != nil {
		return err
	}
	if err := gsettingsSet(schema, "command", command); err != nil {
		return err
	}
	return gsettingsSet(schema, "binding", "<Super>v")
}

func releaseGNOMESuperVConflict() error {
	bindings, err := gsettingsGetArray(gnomeShellKeybindings, gnomeMessageTrayKey)
	if err != nil {
		return nil
	}
	var kept []string
	removed := false
	for _, b := range bindings {
		if strings.EqualFold(b, "<Super>v") {
			removed = true
			continue
		}
		kept = append(kept, b)
	}
	if !removed {
		return nil
	}
	if err := gsettingsSetArray(gnomeShellKeybindings, gnomeMessageTrayKey, kept); err != nil {
		return err
	}
	log.Printf("hotkey: freed %s from GNOME message tray", config.HotkeyLabel)
	return nil
}
