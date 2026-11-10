//go:build linux

package platform

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	pasteTargetMu    sync.Mutex
	pasteTargetID    string
	pasteTargetGnome string
)

// RaiseWindow maps and activates a window by title. xfwm4 often leaves a GLFW
// window unmapped after the first Show; an explicit map makes it visible to
// the window manager.
func RaiseWindow(title string) {
	title = strings.TrimSpace(title)
	if title == "" || os.Getenv("DISPLAY") == "" {
		return
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		return
	}
	time.Sleep(80 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xdotool", "search", "--name", title, "windowmap", "windowactivate")
	if err := cmd.Run(); err != nil {
		log.Printf("raise window %q: %v", title, err)
	}
}

// HasPasteTarget reports whether a foreground window was captured before the popup opened.
func HasPasteTarget() bool {
	pasteTargetMu.Lock()
	defer pasteTargetMu.Unlock()
	return pasteTargetID != "" || pasteTargetGnome != ""
}

// CapturePasteTarget remembers the foreground window (CopyQ-style) before LinBoard takes focus.
func CapturePasteTarget() {
	pasteTargetMu.Lock()
	defer pasteTargetMu.Unlock()
	pasteTargetID = ""
	pasteTargetGnome = ""

	if seq := gnomeCaptureFocus(); seq != "" {
		pasteTargetGnome = seq
		return
	}

	if _, err := exec.LookPath("xdotool"); err != nil {
		return
	}
	out, err := exec.Command("xdotool", "getactivewindow").Output()
	if err != nil {
		return
	}
	id := strings.TrimSpace(string(out))
	if id != "" && id != "0" {
		pasteTargetID = id
	}
}

// RestorePasteTarget raises the window captured before the history popup opened.
func RestorePasteTarget() {
	pasteTargetMu.Lock()
	gnomeSeq := pasteTargetGnome
	xid := pasteTargetID
	pasteTargetMu.Unlock()

	if gnomeSeq != "" {
		gnomeRestoreFocus(gnomeSeq)
		time.Sleep(100 * time.Millisecond)
		return
	}

	if xid == "" {
		return
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		return
	}
	cmd := exec.Command("xdotool", "windowactivate", "--sync", xid)
	if err := cmd.Run(); err != nil {
		log.Printf("windowactivate %s: %v", xid, err)
		return
	}
	time.Sleep(50 * time.Millisecond)
}
