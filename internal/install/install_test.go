package install

import (
	"strings"
	"testing"
)

func TestDesktopExec(t *testing.T) {
	plain := "/home/user/.local/bin/linboard-start"
	if got := desktopExec(plain); got != plain {
		t.Fatalf("got %s", got)
	}
	spaced := desktopExec("/home/a b/linboard")
	if spaced != `"/home/a b/linboard"` {
		t.Fatalf("got %s", spaced)
	}
	body := desktopEntry("/home/a b/linboard", true)
	if !strings.Contains(body, `Exec="/home/a b/linboard"`) {
		t.Fatalf("entry:\n%s", body)
	}
	if !strings.Contains(body, "X-GNOME-Autostart-enabled=true") {
		t.Fatal(body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatal("desktop entry should end with a newline")
	}
}
