package clipboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/foisal/linboard/internal/platform"
)

var pasteHintOnce sync.Once

type pasteShortcut struct {
	name string
	bin  string
	args []string
}

// PasteToTarget restores the previous window and simulates paste (CopyQ-style).
func PasteToTarget() error {
	delay := 150 * time.Millisecond
	if platform.UsePortalHotkey() {
		delay = 280 * time.Millisecond
	}
	time.Sleep(delay)
	platform.RestorePasteTarget()
	time.Sleep(120 * time.Millisecond)
	return simulatePaste()
}

func simulatePaste() error {
	// Built-in uinput works on all Linux desktops (GNOME/KDE/XFCE/X11/Wayland).
	if err := pasteViaUinput(); err == nil {
		return nil
	} else if !errors.Is(err, errUinputUnavailable) {
		log.Printf("auto-paste (uinput) failed: %v", err)
	}

	// X11 and XWayland apps: xdotool after focus restore.
	if useXdotoolPaste() {
		for _, s := range xdotoolShortcuts() {
			if err := runPasteShortcut(s); err == nil {
				return nil
			}
		}
	}

	pasteHintOnce.Do(func() {
		if hint := PasteSessionHint(); hint != "" {
			log.Printf("auto-paste: %s", strings.ReplaceAll(hint, "\n", " "))
		} else {
			log.Printf("auto-paste failed — run: linboard setup-paste")
		}
	})
	return fmt.Errorf("auto-paste unavailable")
}

func useXdotoolPaste() bool {
	if !haveCommand("xdotool") {
		return false
	}
	if !platform.UsePortalHotkey() {
		return true
	}
	return platform.HasPasteTarget()
}

func runPasteShortcut(s pasteShortcut) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, s.args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			log.Printf("auto-paste (%s) failed: %v: %s", s.name, err, detail)
		} else {
			log.Printf("auto-paste (%s) failed: %v", s.name, err)
		}
		return err
	}
	return nil
}

func xdotoolShortcuts() []pasteShortcut {
	return []pasteShortcut{
		{name: "xdotool-ctrl-v", bin: "xdotool", args: []string{"key", "ctrl+v"}},
		{name: "xdotool-shift-insert", bin: "xdotool", args: []string{"key", "shift+Insert"}},
	}
}

// PasteReady reports whether auto-paste should work in this session.
func PasteReady() bool {
	if uinputReady() {
		return true
	}
	return useXdotoolPaste()
}

// PasteSetupHint returns one-time setup instructions when paste is unavailable.
func PasteSetupHint() string {
	return `Linux auto-paste needs /dev/uinput access (all desktops):

sudo tee /etc/udev/rules.d/99-linboard-uinput.rules <<'EOF'
KERNEL=="uinput", GROUP="input", MODE="0660", OPTIONS+="static_node=uinput"
EOF
sudo udevadm control --reload-rules && sudo udevadm trigger
sudo usermod -aG input $USER

Then log out and log back in, and restart LinBoard.

X11-only fallback: sudo apt install xdotool  (or dnf/pacman equivalent)`
}

// PasteToolName reports the primary paste backend for diagnostics.
func PasteToolName() string {
	if uinputReady() {
		return "uinput"
	}
	if SessionNeedsRelogin() {
		return "uinput (log out/in)"
	}
	if UinputWritable() {
		return "uinput"
	}
	if useXdotoolPaste() {
		return "xdotool"
	}
	return "none"
}
