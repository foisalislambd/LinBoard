//go:build !linux

package tray

import "errors"

func xembedTrayPresent() bool { return false }

func startXEmbed(func()) (func(), error) {
	return nil, errors.New("XEmbed tray is only available on Linux")
}
