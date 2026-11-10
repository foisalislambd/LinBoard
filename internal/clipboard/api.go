package clipboard

import (
	"context"
	"errors"
	"fmt"

	"github.com/foisal/linboard/internal/platform"
)

var errFyneNotReady = errors.New("fyne clipboard not configured")

// CopyToolName returns the active clipboard write backend for diagnostics.
func CopyToolName() string {
	return clipboardBackendName()
}

func clipboardBackendName() string {
	if fyneAvailable() {
		return "fyne"
	}
	if haveCommand("xclip") {
		return "xclip"
	}
	if haveCommand("xsel") {
		return "xsel"
	}
	return "none"
}

// ReadToolName reports the backend the monitor actually reads with.
// On X11 that is xclip/xsel. Fyne's GLFW clipboard runs on the UI thread and
// will busy-loop inside the X11 INCR handler on a large image selection.
func ReadToolName() string {
	if !platform.UsePortalHotkey() {
		if haveCommand("xclip") {
			return "xclip"
		}
		if haveCommand("xsel") {
			return "xsel"
		}
		return "none"
	}
	return clipboardBackendName()
}

// WriteText puts UTF-8 text on the system clipboard.
func WriteText(text string) error {
	writers := textWriters()
	if len(writers) == 0 {
		return fmt.Errorf("clipboard write failed")
	}
	var lastErr error
	ok := false
	for _, w := range writers {
		if err := w(text); err != nil {
			lastErr = err
			continue
		}
		ok = true
	}
	if ok {
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("clipboard write failed: %w", lastErr)
	}
	return fmt.Errorf("clipboard write failed")
}

// WriteImage puts PNG bytes on the system clipboard.
func WriteImage(data []byte) error {
	for _, w := range imageWriters() {
		if err := w(data); err == nil {
			return nil
		}
	}
	if platform.UsePortalHotkey() {
		return fmt.Errorf("clipboard image write failed (X11 image tools unavailable on Wayland)")
	}
	return fmt.Errorf("clipboard image write failed (install xclip)")
}

// backendRead is one clipboard backend's result.
// authoritative means an empty string is the real clipboard (do not fall through).
type backendRead struct {
	text          string
	err           error
	authoritative bool
}

// chooseClipboardText picks a clipboard string from backends in order.
// On Wayland the Fyne read is authoritative, including when it is empty, so a
// stale X11/XWayland selection is not treated as the current clipboard.
func chooseClipboardText(reads []backendRead) string {
	for i, r := range reads {
		if r.err != nil {
			continue
		}
		if i == 0 && r.authoritative {
			return r.text
		}
		if r.text != "" {
			return r.text
		}
	}
	return ""
}

func readTextContext(ctx context.Context) (string, error) {
	// X11 reads go through xclip/xsel only. GLFW's getSelectionString runs on
	// the UI thread, and its INCR loop spins at 100% CPU (and never returns)
	// when the selection is a multi-megabyte image.
	if !platform.UsePortalHotkey() {
		if haveCommand("xclip") {
			raw, err := readTargets(ctx)
			if err != nil {
				return "", nil
			}
			if !planReads(raw).ReadText {
				return "", nil
			}
			text, err := readExplicitText(ctx)
			if err != nil {
				return "", err
			}
			return sanitizeText(text)
		}
		if haveCommand("xsel") {
			text, err := readTextXSel(ctx)
			if err != nil {
				return "", nil
			}
			return sanitizeText(text)
		}
		return "", nil
	}

	var reads []backendRead
	if fyneAvailable() {
		text, err := readTextFyne()
		reads = append(reads, backendRead{
			text:          text,
			err:           err,
			authoritative: true,
		})
	}
	if haveCommand("xclip") {
		raw, err := readTargets(ctx)
		if err == nil && planReads(raw).ReadText {
			text, err := readExplicitText(ctx)
			reads = append(reads, backendRead{text: text, err: err})
		}
	}
	text := chooseClipboardText(reads)
	return sanitizeText(text)
}

func readImageContext(ctx context.Context) ([]byte, error) {
	if haveCommand("xclip") {
		if data, err := readImageXClip(ctx); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, fmt.Errorf("clipboard image read failed")
}

func textWriters() []func(string) error {
	// LinBoard stays running in the tray — Fyne/GLFW is the native Wayland path (like Qt in CopyQ).
	writers := []func(string) error{}
	if fyneAvailable() {
		writers = append(writers, writeTextFyne)
	}
	if !platform.UsePortalHotkey() || haveCommand("xclip") {
		writers = append(writers, writeTextXClip, writeTextXSel)
	}
	return writers
}

func imageWriters() []func([]byte) error {
	// Images use X11 tools (XWayland bridge on GNOME Wayland). Pure Fyne API is text-only.
	if haveCommand("xclip") {
		return []func([]byte) error{writeImageXClip}
	}
	return nil
}
