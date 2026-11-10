package hotkey

import (
	"fmt"
	"log"
	"sync"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

type backend interface {
	start(func()) error
	stop()
}

// Manager registers Super+V using the best backend for the session and desktop.
type Manager struct {
	mu      sync.Mutex
	backend backend
	onPress func()
	stopped bool
}

func New() *Manager {
	return &Manager{}
}

func (m *Manager) OnPress(fn func()) {
	m.mu.Lock()
	m.onPress = fn
	m.mu.Unlock()
}

func (m *Manager) Start() error {
	m.mu.Lock()
	if m.onPress == nil {
		m.mu.Unlock()
		return fmt.Errorf("hotkey: no handler")
	}
	if m.stopped {
		m.mu.Unlock()
		return fmt.Errorf("hotkey: stopped")
	}
	onPress := m.onPress
	m.mu.Unlock()

	b, err := openHotkeyBackend(onPress)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		if b != nil {
			b.stop()
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("hotkey: stopped")
	}
	if err != nil {
		return err
	}
	m.backend = b
	return nil
}

func openHotkeyBackend(onPress func()) (backend, error) {
	log.Printf("hotkey: %s on %s, target %s",
		platform.SessionDescription(), platform.DesktopName(), config.HotkeyLabel)

	// 1) xdg-desktop-portal (KDE Plasma 6+, future GNOME)
	if platform.UsePortalHotkey() && portalHasGlobalShortcuts() {
		b := &portalBackend{}
		if err := b.start(onPress); err != nil {
			log.Printf("hotkey: portal failed: %v", err)
		} else {
			return b, nil
		}
	} else if platform.UsePortalHotkey() {
		log.Printf("hotkey: portal GlobalShortcuts not available")
	}

	// 2) Desktop environment system shortcut → linboard toggle
	if b := desktopBackend(); b != nil {
		if err := b.start(onPress); err != nil {
			log.Printf("hotkey: %s shortcut failed: %v", platform.DesktopName(), err)
		} else {
			return b, nil
		}
	}

	// 3) Register Super+V via gsettings/khotkeys/etc. → `linboard toggle` (IPC)
	if regErr := RegisterSystemShortcut(); regErr == nil {
		log.Printf("hotkey registered (system): %s → linboard toggle", config.HotkeyLabel)
		return &noopBackend{}, nil
	} else {
		log.Printf("hotkey: system registration: %v", regErr)
	}

	return nil, fmt.Errorf("could not bind %s — run: linboard install-shortcut", config.HotkeyLabel)
}

func desktopBackend() backend {
	switch platform.CurrentDesktop() {
	case platform.DesktopGNOME:
		return &gnomeBackend{}
	case platform.DesktopKDE:
		return &kdeBackend{}
	case platform.DesktopXFCE:
		return &xfceBackend{}
	case platform.DesktopCinnamon:
		return &cinnamonBackend{}
	case platform.DesktopMATE:
		return &mateBackend{}
	default:
		return nil
	}
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	if m.backend != nil {
		m.backend.stop()
		m.backend = nil
	}
}

type noopBackend struct{}

func (noopBackend) start(func()) error { return nil }
func (noopBackend) stop()              {}
