package hotkey

import (
	"strings"

	"github.com/godbus/dbus/v5"
)

func portalHasGlobalShortcuts() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	var xml string
	obj := conn.Object(portalBusName, dbus.ObjectPath(portalObjectPath))
	if err := obj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml); err != nil {
		return false
	}
	return strings.Contains(xml, portalIface)
}
