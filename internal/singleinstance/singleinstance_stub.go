//go:build !linux

package singleinstance

import "fmt"

// Acquire is unavailable outside Linux — LinBoard targets Linux desktops only.
func Acquire() (func(), error) {
	return nil, fmt.Errorf("LinBoard single-instance lock requires Linux")
}
