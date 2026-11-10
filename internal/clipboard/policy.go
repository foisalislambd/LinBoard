package clipboard

import (
	"errors"
	"io"
	"strings"

	"github.com/foisal/linboard/internal/store"
)

// errBinaryClip means the clipboard bytes are an image or other non-text
// payload. Text history must not store them.
var errBinaryClip = errors.New("clipboard is not text")

// clipPlan is which payloads a TARGETS list allows us to request.
// Text is requested only when a text target is advertised, so an image-only
// selection is never read as text (that race stored PNG bytes as text clips).
type clipPlan struct {
	ReadText  bool
	ReadImage bool
}

func planReads(targets string) clipPlan {
	var plan clipPlan
	for _, line := range strings.Split(targets, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if isTextTarget(t) {
			plan.ReadText = true
		}
		if strings.HasPrefix(t, "image/") {
			plan.ReadImage = true
		}
	}
	return plan
}

func isTextTarget(t string) bool {
	switch t {
	case "UTF8_STRING", "STRING", "TEXT", "COMPOUND_TEXT", "text/plain", "text/plain;charset=utf-8", "text/unicode":
		return true
	default:
		return strings.HasPrefix(t, "text/")
	}
}

// sanitizeText drops empty, oversize, and binary payloads before they can be stored.
func sanitizeText(text string) (string, error) {
	if text == "" {
		return "", nil
	}
	if len(text) > store.MaxTextBytes {
		return "", store.ErrTooLarge
	}
	if binaryText(text) {
		return "", errBinaryClip
	}
	return text, nil
}

func binaryText(s string) bool {
	if strings.HasPrefix(s, "\x89PNG") {
		return true
	}
	if strings.IndexByte(s, 0) >= 0 {
		return true
	}
	n := len(s)
	if n > 512 {
		n = 512
	}
	if n < 16 {
		return false
	}
	bad := 0
	for i := 0; i < n; i++ {
		c := s[i]
		if c < 0x09 || (c > 0x0d && c < 0x20) {
			bad++
		}
	}
	return bad*5 > n
}

// readCapped reads at most max bytes. A longer stream returns store.ErrTooLarge
// without consuming the rest.
func readCapped(r io.Reader, max int) ([]byte, error) {
	if max < 1 {
		return nil, store.ErrTooLarge
	}
	limited := io.LimitReader(r, int64(max)+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(buf) > max {
		return nil, store.ErrTooLarge
	}
	return buf, nil
}
