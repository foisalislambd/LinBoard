package hotkey

import (
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/godbus/dbus/v5"
)

const mediaKeysBus = "org.gnome.SettingsDaemon.MediaKeys"

func mediaKeysRunning() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	var hasOwner bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, mediaKeysBus).Store(&hasOwner)
	return err == nil && hasOwner
}

// ensureGNOMEMediaKeys starts gsd-media-keys when missing (required for custom shortcuts).
func ensureGNOMEMediaKeys() error {
	if mediaKeysRunning() {
		return nil
	}
	path := "/usr/libexec/gsd-media-keys"
	if _, err := os.Stat(path); err != nil {
		if p, err := exec.LookPath("gsd-media-keys"); err == nil {
			path = p
		} else {
			return skip("gsd-media-keys not installed")
		}
	}
	cmd := exec.Command(path)
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	// Detach so an early exit cannot leave a zombie, and so LinBoard's
	// shutdown does not wait on the session daemon.
	_ = cmd.Process.Release()
	for i := 0; i < 30; i++ {
		if mediaKeysRunning() {
			log.Printf("hotkey: started %s", mediaKeysBus)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return skip("gsd-media-keys did not start")
}
