package hotkey

import (
	"strings"
	"testing"

	"github.com/foisal/linboard/internal/platform"
)

func TestVerifyUnknownDesktopIsNotReady(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")
	restore := platform.SetRunningProcessesForTest(func() []string { return nil })
	defer restore()

	r := Verify("/tmp/linboard-not-installed")
	if r.Healthy() {
		t.Fatalf("doctor treated a missing binding as ready: ok=%v warn=%v", r.OK, r.Warn)
	}
	joined := strings.Join(r.Fail, "\n")
	if !strings.Contains(joined, "not registered") {
		t.Fatalf("fail lines: %s", joined)
	}
}

func TestVerifyDetectsXFCEFromProcesses(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")
	restore := platform.SetRunningProcessesForTest(func() []string {
		return []string{"xfce4-panel", "xfwm4"}
	})
	defer restore()
	if platform.CurrentDesktop() != platform.DesktopXFCE {
		t.Fatal(platform.DesktopName())
	}
	r := Verify("/tmp/linboard-not-installed")
	// No xfconf in this environment, so the binding is absent and doctor must not say ready.
	if r.Healthy() {
		t.Fatalf("expected a failed XFCE check, ok=%v warn=%v fail=%v", r.OK, r.Warn, r.Fail)
	}
}
