package clipboard

import "testing"

func TestChooseClipboardText(t *testing.T) {
	waylandEmpty := []backendRead{{
		text:          "",
		authoritative: true,
	}, {
		text: "stale-from-x11",
	}}
	if got := chooseClipboardText(waylandEmpty); got != "" {
		t.Fatalf("wayland empty should win, got %q", got)
	}

	fallback := []backendRead{
		{err: errFyneNotReady, authoritative: true},
		{text: ""},
		{text: "from-xsel"},
	}
	if got := chooseClipboardText(fallback); got != "from-xsel" {
		t.Fatalf("got %q", got)
	}

	first := []backendRead{{text: "fyne"}, {text: "xclip"}}
	if got := chooseClipboardText(first); got != "fyne" {
		t.Fatalf("got %q", got)
	}
}
