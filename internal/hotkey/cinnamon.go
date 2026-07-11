package hotkey

import (
	"fmt"
	"log"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

type cinnamonBackend struct{}

func (b *cinnamonBackend) start(_ func()) error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	if err := setupCinnamonHotkey(exe); err != nil {
		return err
	}
	log.Printf("hotkey registered (Cinnamon): %s → linboard toggle", config.HotkeyLabel)
	return nil
}

func (b *cinnamonBackend) stop() {}

func setupCinnamonHotkey(exe string) error {
	if !platform.IsCinnamon() {
		return skip("not Cinnamon")
	}
	if !hasBin("gsettings") {
		return fmt.Errorf("gsettings not found")
	}

	listSchema := "org.cinnamon.desktop.keybindings"
	path := "/org/cinnamon/desktop/keybindings/custom-keybindings/custom-linboard/"
	fullSchema := listSchema + ".custom-keybinding:" + path

	paths, err := gsettingsGetArray(listSchema, "custom-keybindings")
	if err != nil || len(paths) == 0 {
		paths, err = gsettingsGetArray(listSchema, "custom-list")
		if err != nil {
			return err
		}
	}
	if !containsPath(paths, path) {
		paths = append(paths, path)
		if err := gsettingsSetArray(listSchema, "custom-keybindings", paths); err != nil {
			if err2 := gsettingsSetArray(listSchema, "custom-list", paths); err2 != nil {
				return err
			}
		}
	}

	if err := gsettingsSet(fullSchema, "name", "LinBoard"); err != nil {
		return err
	}
	if err := gsettingsSet(fullSchema, "command", exe+" toggle"); err != nil {
		return err
	}
	return gsettingsSet(fullSchema, "binding", "<Super>v")
}
