package platform

import "testing"

func TestCurrentDesktop(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "ubuntu")
	if CurrentDesktop() != DesktopGNOME || DesktopName() != "GNOME" {
		t.Fatalf("got %v %s", CurrentDesktop(), DesktopName())
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	t.Setenv("DESKTOP_SESSION", "plasmax11")
	if CurrentDesktop() != DesktopKDE {
		t.Fatal(CurrentDesktop())
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "XFCE")
	t.Setenv("DESKTOP_SESSION", "")
	if CurrentDesktop() != DesktopXFCE {
		t.Fatal(CurrentDesktop())
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "X-Cinnamon")
	if CurrentDesktop() != DesktopCinnamon {
		t.Fatal(CurrentDesktop())
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "MATE")
	if CurrentDesktop() != DesktopMATE {
		t.Fatal(CurrentDesktop())
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "Hyprland")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "hyprland")
	if CurrentDesktop() != DesktopOther {
		t.Fatal(CurrentDesktop())
	}
}

func TestDesktopFallbackWhenEnvUnset(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("XDG_SESSION_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")

	restore := SetRunningProcessesForTest(func() []string {
		return []string{"xfce4-panel", "xfconfd", "xfwm4"}
	})
	defer restore()
	if CurrentDesktop() != DesktopXFCE || DesktopName() != "XFCE" {
		t.Fatalf("xfce processes: %v %s", CurrentDesktop(), DesktopName())
	}

	restore()
	restore = SetRunningProcessesForTest(func() []string { return []string{"gnome-shell"} })
	defer restore()
	if CurrentDesktop() != DesktopGNOME {
		t.Fatal(CurrentDesktop())
	}

	restore()
	restore = SetRunningProcessesForTest(func() []string { return []string{"plasmashell"} })
	defer restore()
	if CurrentDesktop() != DesktopKDE {
		t.Fatal(CurrentDesktop())
	}

	restore()
	restore = SetRunningProcessesForTest(func() []string { return []string{"cinnamon-sessio"} })
	defer restore()
	if CurrentDesktop() != DesktopCinnamon {
		t.Fatal(CurrentDesktop())
	}

	restore()
	restore = SetRunningProcessesForTest(func() []string { return []string{"mate-panel"} })
	defer restore()
	if CurrentDesktop() != DesktopMATE {
		t.Fatal(CurrentDesktop())
	}

	// An explicit unknown desktop is not overridden by whatever happens to be running.
	t.Setenv("XDG_CURRENT_DESKTOP", "Hyprland")
	if CurrentDesktop() != DesktopOther {
		t.Fatal(CurrentDesktop())
	}
}

func TestSessionDesktopEnv(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")
	t.Setenv("XDG_SESSION_DESKTOP", "xfce")
	if CurrentDesktop() != DesktopXFCE {
		t.Fatal(CurrentDesktop())
	}
}

func TestUsePortalHotkey(t *testing.T) {
	t.Setenv("LINBOARD_FORCE_X11_HOTKEY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	if !UsePortalHotkey() || SessionDescription() != "wayland" {
		t.Fatal("expected wayland")
	}
	t.Setenv("XDG_SESSION_TYPE", "x11")
	if UsePortalHotkey() || SessionDescription() != "x11" {
		t.Fatal("expected x11")
	}
	t.Setenv("LINBOARD_FORCE_X11_HOTKEY", "1")
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	if UsePortalHotkey() {
		t.Fatal("force x11 should win")
	}
	t.Setenv("LINBOARD_FORCE_X11_HOTKEY", "")
	t.Setenv("XDG_SESSION_TYPE", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if !UsePortalHotkey() {
		t.Fatal("wayland display without x11 should use portal")
	}
}
