package hotkey

import (
	"fmt"
	"log"
	"strings"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

const (
	mateKeybindingSchema = "org.mate.control-center.keybinding"
	mateBindingPath      = "/org/mate/desktop/keybindings/custom-linboard/"
)

type mateBackend struct{}

func (b *mateBackend) start(_ func()) error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	if err := setupMATEHotkey(exe); err != nil {
		return err
	}
	log.Printf("hotkey registered (MATE): %s → linboard toggle", config.HotkeyLabel)
	return nil
}

func (b *mateBackend) stop() {}

func mateBindingFullSchema() string {
	return mateKeybindingSchema + ":" + mateBindingPath
}

func setupMATEHotkey(exe string) error {
	if !platform.IsMATE() {
		return skip("not MATE")
	}
	if !hasBin("gsettings") {
		return fmt.Errorf("gsettings not found")
	}

	schema := mateBindingFullSchema()
	if err := gsettingsSet(schema, "name", "LinBoard"); err != nil {
		return err
	}
	if err := gsettingsSet(schema, "action", exe+" toggle"); err != nil {
		return err
	}
	return gsettingsSet(schema, "binding", "<Super>v")
}

func verifyMATEShortcut(r VerifyReport, exe string) VerifyReport {
	r.OK = append(r.OK, "desktop: MATE")
	if !hasBin("gsettings") {
		r.Fail = append(r.Fail, "gsettings not found")
		return r
	}
	schema := mateBindingFullSchema()
	action, err := gsettingsCommand(schema, "action")
	if err != nil {
		r.Fail = append(r.Fail, "MATE shortcut not registered")
		return r
	}
	if action != exe+" toggle" {
		r.Warn = append(r.Warn, fmt.Sprintf("MATE action is %q (want %q)", action, exe+" toggle"))
	} else {
		r.OK = append(r.OK, "MATE action OK")
	}
	bind, err := gsettingsGet(schema, "binding")
	if err != nil {
		r.Fail = append(r.Fail, "MATE shortcut binding not set")
	} else if strings.Trim(bind, "'") != "<Super>v" {
		r.Warn = append(r.Warn, "binding is "+bind+" (want <Super>v)")
	} else {
		r.OK = append(r.OK, "binding: Super+V")
	}
	return r
}
