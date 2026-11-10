//go:build !linux

package clipboard

import "context"

func clipboardStamp(context.Context) (uint64, bool, error) {
	return 0, false, nil
}
