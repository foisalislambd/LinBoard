package platform

import (
	"os"
	"strings"
)

type Desktop int

const (
	DesktopGNOME Desktop = iota
	DesktopKDE
	DesktopXFCE
	DesktopCinnamon
	DesktopMATE
	DesktopOther
)

// runningProcesses lists comm names of running processes. Tests replace it.
var runningProcesses = listProcComm

// SetRunningProcessesForTest replaces process detection and returns a restore func.
func SetRunningProcessesForTest(fn func() []string) func() {
	prev := runningProcesses
	if fn == nil {
		runningProcesses = listProcComm
	} else {
		runningProcesses = fn
	}
	return func() { runningProcesses = prev }
}

func CurrentDesktop() Desktop {
	envs := []string{
		os.Getenv("XDG_CURRENT_DESKTOP"),
		os.Getenv("XDG_SESSION_DESKTOP"),
		os.Getenv("DESKTOP_SESSION"),
	}
	saw := false
	for _, env := range envs {
		env = strings.TrimSpace(env)
		if env == "" {
			continue
		}
		saw = true
		if d := matchDesktop(env); d != DesktopOther {
			return d
		}
	}
	// Custom sessions (Xfwm4 + xfce4-panel) often leave every XDG desktop
	// variable unset. Fall back to the processes that are actually running,
	// but do not override an explicit unknown desktop name.
	if !saw {
		if d := desktopFromProcesses(runningProcesses()); d != DesktopOther {
			return d
		}
	}
	return DesktopOther
}

func matchDesktop(v string) Desktop {
	v = strings.ToLower(v)
	switch {
	case strings.Contains(v, "kde") || strings.Contains(v, "plasma"):
		return DesktopKDE
	case strings.Contains(v, "xfce"):
		return DesktopXFCE
	case strings.Contains(v, "cinnamon"):
		return DesktopCinnamon
	case strings.Contains(v, "mate"):
		return DesktopMATE
	case strings.Contains(v, "gnome") || strings.Contains(v, "ubuntu") || strings.Contains(v, "unity"):
		return DesktopGNOME
	default:
		return DesktopOther
	}
}

func desktopFromProcesses(names []string) Desktop {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[strings.ToLower(strings.TrimSpace(n))] = struct{}{}
	}
	has := func(names ...string) bool {
		for _, n := range names {
			if _, ok := set[n]; ok {
				return true
			}
			// /proc/<pid>/comm keeps 15 characters (TASK_COMM_LEN includes NUL).
			if len(n) > 15 {
				if _, ok := set[n[:15]]; ok {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("xfce4-panel", "xfconfd", "xfwm4"):
		return DesktopXFCE
	case has("plasmashell", "kwin_x11", "kwin_wayland"):
		return DesktopKDE
	case has("cinnamon", "cinnamon-session", "cinnamon-sessio"):
		return DesktopCinnamon
	case has("mate-panel", "mate-session"):
		return DesktopMATE
	case has("gnome-shell"):
		return DesktopGNOME
	default:
		return DesktopOther
	}
}

func listProcComm() []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	names := make([]string, 0, 64)
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "" || e.Name()[0] < '1' || e.Name()[0] > '9' {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		names = append(names, strings.TrimSpace(string(b)))
	}
	return names
}

func DesktopName() string {
	switch CurrentDesktop() {
	case DesktopGNOME:
		return "GNOME"
	case DesktopKDE:
		return "KDE Plasma"
	case DesktopXFCE:
		return "XFCE"
	case DesktopCinnamon:
		return "Cinnamon"
	case DesktopMATE:
		return "MATE"
	default:
		return "Linux"
	}
}

func IsGNOME() bool    { return CurrentDesktop() == DesktopGNOME }
func IsKDE() bool      { return CurrentDesktop() == DesktopKDE }
func IsXFCE() bool     { return CurrentDesktop() == DesktopXFCE }
func IsCinnamon() bool { return CurrentDesktop() == DesktopCinnamon }
func IsMATE() bool     { return CurrentDesktop() == DesktopMATE }
