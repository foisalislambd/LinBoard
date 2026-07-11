package singleinstance

import "errors"

// ErrAlreadyRunning is returned when another LinBoard instance holds the lock.
var ErrAlreadyRunning = errors.New("LinBoard is already running")
