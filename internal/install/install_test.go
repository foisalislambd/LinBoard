package install

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(body, "X-GNOME-Autostart-Delay=2") {
		t.Fatal(body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Fatal("desktop entry should end with a newline")
	}
}

func TestLauncherStartsWithoutSG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "linboard-start")
	if err := writeLauncher(path, "/usr/bin/linboard"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, "exec sg ") {
		t.Fatalf("sg must not replace the launcher, or a missing sg never starts LinBoard:\n%s", text)
	}
	if !strings.Contains(text, `exec "$LB" "$@"`) {
		t.Fatalf("launcher must exec the binary when sg is unavailable:\n%s", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("launcher is not executable: %s", info.Mode())
	}
}
