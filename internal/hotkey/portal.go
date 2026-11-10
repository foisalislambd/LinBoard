package hotkey

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/foisal/linboard/internal/config"
	"github.com/godbus/dbus/v5"
)

const (
	portalBusName    = "org.freedesktop.portal.Desktop"
	portalObjectPath = "/org/freedesktop/portal/desktop"
	portalIface      = "org.freedesktop.portal.GlobalShortcuts"
	requestIface     = "org.freedesktop.portal.Request"
	shortcutID       = "linboard_toggle"
)

type portalBackend struct {
	conn     *dbus.Conn
	session  dbus.ObjectPath
	onPress  func()
	stopCh   chan struct{}
	stopOnce sync.Once
	sigCh    chan *dbus.Signal
}

func (b *portalBackend) start(onPress func()) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("session bus: %w", err)
	}
	b.conn = conn
	b.onPress = onPress
	b.stopCh = make(chan struct{})
	b.sigCh = make(chan *dbus.Signal, 8)
	conn.Signal(b.sigCh)

	session, err := b.createSession()
	if err != nil {
		_ = conn.Close()
		return err
	}
	b.session = session
	if err := b.bindShortcuts(session); err != nil {
		_ = conn.Close()
		return err
	}
	if err := b.watchActivated(); err != nil {
		_ = conn.Close()
		return err
	}

	log.Printf("hotkey registered (portal): %s — configure in Settings → Keyboard if prompted", config.HotkeyLabel)
	return nil
}

func (b *portalBackend) createSession() (dbus.ObjectPath, error) {
	opts := map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(randomToken()),
		"session_handle_token": dbus.MakeVariant(randomToken()),
	}
	return b.portalRequest(portalIface+".CreateSession", 30*time.Second, opts)
}

func (b *portalBackend) bindShortcuts(session dbus.ObjectPath) error {
	// Portal expects a(sa{sv}): array of (shortcut_id, options).
	// A slice of structs encodes as that signature; []interface{} does not.
	shortcuts := []struct {
		ID      string
		Options map[string]dbus.Variant
	}{
		{
			ID: shortcutID,
			Options: map[string]dbus.Variant{
				"description":       dbus.MakeVariant(config.AppName),
				"preferred_trigger": dbus.MakeVariant("LOGO+V"),
			},
		},
	}
	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(randomToken()),
	}
	return b.portalVoid(
		portalIface+".BindShortcuts",
		60*time.Second,
		session, shortcuts, "", opts,
	)
}

func (b *portalBackend) portalRequest(method string, timeout time.Duration, args ...interface{}) (dbus.ObjectPath, error) {
	return b.portalCall(method, timeout, true, args...)
}

func (b *portalBackend) portalVoid(method string, timeout time.Duration, args ...interface{}) error {
	_, err := b.portalCall(method, timeout, false, args...)
	return err
}

func (b *portalBackend) portalCall(method string, timeout time.Duration, wantSession bool, args ...interface{}) (dbus.ObjectPath, error) {
	obj := b.conn.Object(portalBusName, dbus.ObjectPath(portalObjectPath))
	var request dbus.ObjectPath
	if err := obj.Call(method, 0, args...).Store(&request); err != nil {
		return "", err
	}

	status, results, err := waitPortalResponse(b.conn, b.sigCh, request, timeout)
	if err != nil {
		return "", fmt.Errorf("%s: %w", method, err)
	}
	if status != 0 {
		return "", fmt.Errorf("%s failed (status %d)", method, status)
	}
	if !wantSession {
		return "", nil
	}
	handle, ok := variantObjectPath(results["session_handle"])
	if !ok {
		return "", fmt.Errorf("%s: missing session_handle", method)
	}
	return handle, nil
}

func waitPortalResponse(conn *dbus.Conn, sigCh chan *dbus.Signal, request dbus.ObjectPath, timeout time.Duration) (uint32, map[string]dbus.Variant, error) {
	rule := fmt.Sprintf("type='signal',path='%s',interface='%s',member='Response'", request, requestIface)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return 2, nil, err
	}
	defer func() { _ = conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule).Err }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return 2, nil, fmt.Errorf("portal request timed out")
		case sig := <-sigCh:
			if sig == nil || sig.Path != request || sig.Name != requestIface+".Response" {
				continue
			}
			if len(sig.Body) < 2 {
				continue
			}
			status, _ := sig.Body[0].(uint32)
			results, _ := sig.Body[1].(map[string]dbus.Variant)
			return status, results, nil
		}
	}
}

func variantObjectPath(v dbus.Variant) (dbus.ObjectPath, bool) {
	if v.Value() == nil {
		return "", false
	}
	if v.Signature().String() == "o" {
		if p, ok := v.Value().(dbus.ObjectPath); ok {
			return p, true
		}
	}
	if s, ok := v.Value().(string); ok && len(s) > 0 && s[0] == '/' {
		return dbus.ObjectPath(s), true
	}
	return "", false
}

func (b *portalBackend) watchActivated() error {
	rule := fmt.Sprintf(
		"type='signal',interface='%s',member='Activated',path='%s'",
		portalIface, portalObjectPath,
	)
	if err := b.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return err
	}
	go b.readActivated()
	return nil
}

func (b *portalBackend) readActivated() {
	for {
		select {
		case <-b.stopCh:
			return
		case sig := <-b.sigCh:
			if sig == nil || sig.Name != portalIface+".Activated" {
				continue
			}
			if len(sig.Body) < 2 {
				continue
			}
			id, _ := sig.Body[1].(string)
			if id == shortcutID && b.onPress != nil {
				b.onPress()
			}
		}
	}
}

func (b *portalBackend) stop() {
	b.stopOnce.Do(func() {
		if b.stopCh != nil {
			close(b.stopCh)
		}
		if b.conn != nil && b.session != "" {
			obj := b.conn.Object(portalBusName, b.session)
			_ = obj.Call("org.freedesktop.portal.Session.Close", 0).Err
		}
		if b.conn != nil {
			_ = b.conn.Close()
		}
	})
}

func randomToken() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return "linboard_" + hex.EncodeToString(buf)
}
