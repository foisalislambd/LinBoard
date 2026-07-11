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

func CurrentDesktop() Desktop {
	desktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	session := strings.ToLower(os.Getenv("DESKTOP_SESSION"))

	switch {
	case strings.Contains(desktop, "kde") || strings.Contains(session, "plasma"):
		return DesktopKDE
	case strings.Contains(desktop, "xfce"):
		return DesktopXFCE
	case strings.Contains(desktop, "cinnamon"):
		return DesktopCinnamon
	case strings.Contains(desktop, "mate"):
		return DesktopMATE
	case strings.Contains(desktop, "gnome") || strings.Contains(desktop, "ubuntu") || strings.Contains(session, "ubuntu"):
		return DesktopGNOME
	default:
		return DesktopOther
	}
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
func IsCinnamon() bool  { return CurrentDesktop() == DesktopCinnamon }
func IsMATE() bool      { return CurrentDesktop() == DesktopMATE }
