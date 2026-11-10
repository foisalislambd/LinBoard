package hotkey

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestPortalShortcutSignature(t *testing.T) {
	sample := []struct {
		ID      string
		Options map[string]dbus.Variant
	}{{
		ID: shortcutID,
		Options: map[string]dbus.Variant{
			"description": dbus.MakeVariant("LinBoard"),
		},
	}}
	sig := dbus.SignatureOf(sample).String()
	if sig != "a(sa{sv})" {
		t.Fatalf("signature %s, want a(sa{sv})", sig)
	}
}
