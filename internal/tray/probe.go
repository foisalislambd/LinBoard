package tray

import (
	"github.com/godbus/dbus/v5"

	"github.com/foisal/linboard/internal/config"
)

// Probe describes whether a panel can show a tray icon.
// okLine is set when an icon can be shown; warnLine when the app will run without one.
func Probe() (okLine, warnLine string) {
	if statusNotifierAvailable() {
		return "tray: StatusNotifier watcher present", ""
	}
	if xembedTrayPresent() {
		return "tray: XEmbed system tray present (no StatusNotifier watcher)", ""
	}
	return "", "tray icon unavailable: no StatusNotifier watcher and no XEmbed tray. Use " + config.HotkeyLabel + " or `linboard toggle`"
}

func statusNotifierAvailable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&owned)
	return err == nil && owned
}
