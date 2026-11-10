//go:build linux

package clipboard

import (
	"testing"
	"unsafe"
)

func TestUinputLayouts(t *testing.T) {
	ev := inputEvent{Type: evKey, Code: keyV, Value: 1}
	if ev.Type != evKey || ev.Code != keyV || ev.Value != 1 {
		t.Fatalf("event %+v", ev)
	}
	if unsafe.Sizeof(ev) != 24 {
		t.Fatalf("input_event size %d, want 24", unsafe.Sizeof(ev))
	}
	if unsafe.Offsetof(ev.Type) != 16 || unsafe.Offsetof(ev.Code) != 18 || unsafe.Offsetof(ev.Value) != 20 {
		t.Fatalf("input_event offsets type=%d code=%d value=%d", unsafe.Offsetof(ev.Type), unsafe.Offsetof(ev.Code), unsafe.Offsetof(ev.Value))
	}
	var setup uinputSetup
	if unsafe.Sizeof(setup) != 92 {
		t.Fatalf("uinput_setup size %d, want 92", unsafe.Sizeof(setup))
	}
	if unsafe.Offsetof(setup.FFEffectsMax) != 88 {
		t.Fatalf("ff_effects_max offset %d", unsafe.Offsetof(setup.FFEffectsMax))
	}
	encoded := uintptr((uiDevSetup >> 16) & 0x3fff)
	if encoded != unsafe.Sizeof(setup) {
		t.Fatalf("UI_DEV_SETUP encodes size %d", encoded)
	}
}
