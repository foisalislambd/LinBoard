package clipboard

import (
	"errors"
	"fmt"

	"github.com/foisal/linboard/internal/platform"
)

var errFyneNotReady = errors.New("fyne clipboard not configured")

// ReadToolName returns the active clipboard read backend for diagnostics.
func ReadToolName() string {
	return clipboardBackendName()
}

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

func readText() (string, error) {
	readers := textReaders()
	for i, r := range readers {
		text, err := r()
		if err != nil {
			continue
		}
		// On Wayland, trust Fyne/GLFW for empty clipboard — do not fall through to stale X11 data.
		if i == 0 && fyneAvailable() && platform.UsePortalHotkey() {
			return text, nil
		}
		if text != "" {
			return text, nil
		}
	}
	return "", nil
}

func readImage() ([]byte, error) {
	for _, r := range imageReaders() {
		if data, err := r(); err == nil && len(data) > 0 {
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

func textReaders() []func() (string, error) {
	readers := []func() (string, error){}
	if fyneAvailable() {
		readers = append(readers, readTextFyne)
	}
	if !platform.UsePortalHotkey() || haveCommand("xclip") {
		readers = append(readers, readTextXClip, readTextXSel)
	}
	return readers
}

func imageReaders() []func() ([]byte, error) {
	if haveCommand("xclip") {
		return []func() ([]byte, error){readImageXClip}
	}
	return nil
}
