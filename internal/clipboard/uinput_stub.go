//go:build !linux

package clipboard

import "errors"

var errUinputUnavailable = errors.New("uinput unavailable")

func uinputReady() bool { return false }

func pasteViaUinput() error { return errUinputUnavailable }

// CloseUinput is a no-op outside Linux.
func CloseUinput() {}
