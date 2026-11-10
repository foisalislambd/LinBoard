package hotkey

import "testing"

func TestGVariantString(t *testing.T) {
	if got := gvariantString("<Super>v"); got != `"<Super>v"` {
		t.Fatalf("got %s", got)
	}
	if got := gvariantString(`a"b\c`); got != `"a\"b\\c"` {
		t.Fatalf("got %s", got)
	}
}

func TestParseGSettingsStringArray(t *testing.T) {
	got := parseGSettingsStringArray(`@as ['/org/a/', '/org/b/']`)
	if len(got) != 2 || got[0] != "/org/a/" || got[1] != "/org/b/" {
		t.Fatalf("%#v", got)
	}
	got = parseGSettingsStringArray(`["custom-linboard", "custom0"]`)
	if len(got) != 2 || got[0] != "custom-linboard" || got[1] != "custom0" {
		t.Fatalf("%#v", got)
	}
	if parseGSettingsStringArray("@as []") != nil {
		t.Fatal("empty")
	}
	if got := unquoteGVariantString(`'<Super>v'`); got != "<Super>v" {
		t.Fatalf("unquote %q", got)
	}
}

func TestPreferBindingListKey(t *testing.T) {
	if got := preferBindingListKey([]string{"custom-keybindings", "custom-list"}); got != "custom-list" {
		t.Fatalf("got %s", got)
	}
	if got := preferBindingListKey([]string{"custom-keybindings"}); got != "custom-keybindings" {
		t.Fatalf("got %s", got)
	}
	if got := preferBindingListKey(nil); got != "" {
		t.Fatalf("got %s", got)
	}
}

func TestMATESchema(t *testing.T) {
	if mateListSchema != "org.mate.desktop.keybindings" || mateListKey != "custom-list" {
		t.Fatalf("unexpected list %s %s", mateListSchema, mateListKey)
	}
	if mateBindingFullSchema() != "org.mate.desktop.keybindings.custom-keybinding:/org/mate/desktop/keybindings/custom-linboard/" {
		t.Fatal(mateBindingFullSchema())
	}
}
