package clipboard

import "testing"

func TestChooseClipboardText(t *testing.T) {
	emptyFyne := []backendRead{{
		text:          "",
		authoritative: true,
	}, {
		text: "from-xwayland",
	}}
	if got := chooseClipboardText(emptyFyne, "older"); got != "from-xwayland" {
		t.Fatalf("empty fyne must not hide a new bridge text, got %q", got)
	}
	if got := chooseClipboardText([]backendRead{{text: "", authoritative: true}}, ""); got != "" {
		t.Fatalf("all-empty clipboard should stay empty, got %q", got)
	}

	stuck := []backendRead{{text: "older", authoritative: true}, {text: "fresh"}}
	if got := chooseClipboardText(stuck, "older"); got != "fresh" {
		t.Fatalf("fyne stuck on the previous clip must not hide the bridge, got %q", got)
	}

	freshFyne := []backendRead{{text: "fresh", authoritative: true}, {text: "stale-x11"}}
	if got := chooseClipboardText(freshFyne, "older"); got != "fresh" {
		t.Fatalf("a new fyne value must win over a stale bridge, got %q", got)
	}

	fallback := []backendRead{
		{err: errFyneNotReady, authoritative: true},
		{text: ""},
		{text: "from-xsel"},
	}
	if got := chooseClipboardText(fallback, ""); got != "from-xsel" {
		t.Fatalf("got %q", got)
	}

	first := []backendRead{{text: "fyne"}, {text: "xclip"}}
	if got := chooseClipboardText(first, ""); got != "fyne" {
		t.Fatalf("got %q", got)
	}
}
