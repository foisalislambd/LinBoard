package clipboard

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/foisal/linboard/internal/store"
)

func TestPlanReadsImageDoesNotRequestText(t *testing.T) {
	plan := planReads("TARGETS\nimage/png\nimage/bmp\n")
	if plan.ReadText {
		t.Fatal("image-only clipboard must not be read as text")
	}
	if !plan.ReadImage {
		t.Fatal("expected image target")
	}

	both := planReads("UTF8_STRING\nimage/png\n")
	if !both.ReadText || !both.ReadImage {
		t.Fatalf("both targets: %+v", both)
	}

	text := planReads("TIMESTAMP\nTARGETS\nUTF8_STRING\nSTRING\ntext/plain\n")
	if !text.ReadText || text.ReadImage {
		t.Fatalf("text targets: %+v", text)
	}

	empty := planReads("")
	if empty.ReadText || empty.ReadImage {
		t.Fatalf("empty: %+v", empty)
	}
}

func TestSanitizeRejectsPNGAndOversize(t *testing.T) {
	png := "\x89PNG\r\n\x1a\ntruncated"
	if _, err := sanitizeText(png); !errors.Is(err, errBinaryClip) {
		t.Fatalf("png text: %v", err)
	}
	if _, err := sanitizeText("ok\x00more"); !errors.Is(err, errBinaryClip) {
		t.Fatalf("nul: %v", err)
	}
	huge := strings.Repeat("a", store.MaxTextBytes+1)
	if _, err := sanitizeText(huge); !errors.Is(err, store.ErrTooLarge) {
		t.Fatalf("oversize: %v", err)
	}
	got, err := sanitizeText("hello")
	if err != nil || got != "hello" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestReadCappedStopsAtLimit(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 1000)
	got, err := readCapped(bytes.NewReader(payload), 32)
	if !errors.Is(err, store.ErrTooLarge) {
		t.Fatalf("err %v", err)
	}
	if got != nil {
		t.Fatalf("kept %d bytes", len(got))
	}

	exact := bytes.Repeat([]byte("y"), 32)
	got, err = readCapped(bytes.NewReader(exact), 32)
	if err != nil || !bytes.Equal(got, exact) {
		t.Fatalf("exact: %d %v", len(got), err)
	}
}
