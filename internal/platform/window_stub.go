//go:build !linux

package platform

func CapturePasteTarget() {}

func RestorePasteTarget() {}

func HasPasteTarget() bool { return false }

func RaiseWindow(string) {}
