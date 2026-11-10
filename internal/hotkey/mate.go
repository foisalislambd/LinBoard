package hotkey

import (
	"fmt"
	"log"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

const (
	mateListSchema  = "org.mate.desktop.keybindings"
	mateListKey     = "custom-list"
	mateEntryID     = "custom-linboard"
	mateEntrySchema = "org.mate.desktop.keybindings.custom-keybinding"
	mateEntryBase   = "/org/mate/desktop/keybindings/"
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

func mateEntryPath() string {
	return mateEntryBase + mateEntryID + "/"
}

func mateBindingFullSchema() string {
	return mateEntrySchema + ":" + mateEntryPath()
}

func setupMATEHotkey(exe string) error {
	if !platform.IsMATE() {
		return skip("not MATE")
	}
	if !hasBin("gsettings") {
		return fmt.Errorf("gsettings not found")
	}

	ids, err := gsettingsGetArray(mateListSchema, mateListKey)
	if err != nil {
		return err
	}
	if !containsPath(ids, mateEntryID) {
		ids = append(ids, mateEntryID)
		if err := gsettingsSetArray(mateListSchema, mateListKey, ids); err != nil {
			return err
		}
	}

	schema := mateBindingFullSchema()
	if err := gsettingsSetString(schema, "name", "LinBoard"); err != nil {
		return err
	}
	if err := gsettingsSetString(schema, "action", exe+" toggle"); err != nil {
		return err
	}
	return gsettingsSetString(schema, "binding", "<Super>v")
}

func verifyMATEShortcut(r VerifyReport, exe string) VerifyReport {
	r.OK = append(r.OK, "desktop: MATE")
	if !hasBin("gsettings") {
		r.Fail = append(r.Fail, "gsettings not found")
		return r
	}
	ids, err := gsettingsGetArray(mateListSchema, mateListKey)
	if err != nil || !containsPath(ids, mateEntryID) {
		r.Fail = append(r.Fail, "custom-linboard not in MATE custom-list")
		return r
	}
	r.OK = append(r.OK, "registered in MATE custom-list")

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
	} else if unquoteGVariantString(bind) != "<Super>v" {
		r.Warn = append(r.Warn, "binding is "+bind+" (want <Super>v)")
	} else {
		r.OK = append(r.OK, "binding: Super+V")
	}
	return r
}
