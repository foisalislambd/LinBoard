//go:build !linux

package clipboard

import "context"

func clipboardStamp(context.Context) (uint64, bool, error) {
	return 0, false, nil
}

func peekClipboard(context.Context) (string, bool, bool) {
	return "", false, false
}
