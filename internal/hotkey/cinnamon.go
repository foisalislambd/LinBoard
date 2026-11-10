package hotkey

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

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

	listKey, err := cinnamonBindingListKey()
	if err != nil {
		return err
	}
	paths, err := gsettingsGetArray(listSchema, listKey)
	if err != nil {
		return err
	}
	if !containsPath(paths, path) {
		paths = append(paths, path)
		if err := gsettingsSetArray(listSchema, listKey, paths); err != nil {
			return err
		}
	}

	if err := gsettingsSetString(fullSchema, "name", "LinBoard"); err != nil {
		return err
	}
	if err := gsettingsSetString(fullSchema, "command", exe+" toggle"); err != nil {
		return err
	}
	return gsettingsSetString(fullSchema, "binding", "<Super>v")
}

// preferBindingListKey picks the Cinnamon gsettings key that holds custom shortcuts.
// custom-list is the key current Cinnamon actually reads; custom-keybindings is older.
func preferBindingListKey(keys []string) string {
	have := make(map[string]bool, len(keys))
	for _, k := range keys {
		have[k] = true
	}
	for _, k := range []string{"custom-list", "custom-keybindings"} {
		if have[k] {
			return k
		}
	}
	return ""
}

func cinnamonBindingListKey() (string, error) {
	out, err := exec.Command("gsettings", "list-keys", "org.cinnamon.desktop.keybindings").Output()
	if err != nil {
		return "", err
	}
	key := preferBindingListKey(strings.Fields(string(out)))
	if key == "" {
		return "", fmt.Errorf("cinnamon keybinding list not found")
	}
	return key, nil
}
