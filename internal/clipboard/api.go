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
// Fyne's GLFW clipboard runs on the UI thread and busy-loops inside the
// selection handler on a large HTML or image offer, so reads go through
// xclip/xsel whenever those tools exist.
func ReadToolName() string {
	if haveCommand("xclip") {
		return "xclip"
	}
	if !platform.UsePortalHotkey() && haveCommand("xsel") {
		return "xsel"
	}
	if fyneAvailable() {
		return "fyne"
	}
	return "none"
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
// The first read is the Fyne/GLFW clipboard when authoritative is set.
// Later reads are the XWayland bridge (xclip).
type backendRead struct {
	text          string
	err           error
	authoritative bool
}

// chooseClipboardText picks the clipboard string that actually changed.
// previous is the last text LinBoard stored. A Fyne value that still equals
// previous must not hide a new bridge value: after this app writes the
// clipboard, GLFW often keeps returning that string while another client has
// already copied something else. A new Fyne value still wins over the bridge,
// so a stale X11 selection is not stored in its place.
func chooseClipboardText(reads []backendRead, previous string) string {
	var fyne string
	var haveFyne bool
	var bridge string
	for _, r := range reads {
		if r.err != nil {
			continue
		}
		if r.authoritative && !haveFyne {
			fyne = r.text
			haveFyne = true
			continue
		}
		if bridge == "" && r.text != "" {
			bridge = r.text
		}
	}
	if !haveFyne {
		return bridge
	}
	if fyne != "" && fyne != previous {
		return fyne
	}
	if bridge != "" && bridge != previous {
		return bridge
	}
	if fyne != "" {
		return fyne
	}
	return bridge
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
			text, err := readExplicitText(ctx, raw)
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

	// GLFW's clipboard read runs on the UI thread and does not return when the
	// offer is large. xclip is capped and timed out, so it is the read path
	// whenever it is installed.
	if haveCommand("xclip") {
		raw, err := readTargets(ctx)
		if err != nil {
			return "", nil
		}
		if !planReads(raw).ReadText {
			return "", nil
		}
		text, err := readExplicitText(ctx, raw)
		if err != nil {
			return "", err
		}
		return sanitizeText(text)
	}

	if fyneAvailable() {
		text, err := readTextFyne()
		if err != nil {
			return "", nil
		}
		return sanitizeText(text)
	}
	return "", nil
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
