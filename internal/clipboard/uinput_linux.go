//go:build linux

package clipboard

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	evKey       = 0x01
	evSyn       = 0x00
	synReport   = 0
	keyLeftCtrl = 29
	keyV        = 47

	uiSetEvbit   = 0x40045564 // _IOW('U', 100, int)
	uiSetKeybit  = 0x40045565 // _IOW('U', 101, int)
	uiDevSetup   = 0x405c5503 // _IOW('U', 3, struct uinput_setup)
	uiDevCreate  = 0x5501     // _IO('U', 1)
	uiDevDestroy = 0x5502     // _IO('U', 2)
)

var errUinputUnavailable = errors.New("uinput unavailable")

type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

type inputID struct {
	Bustype uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputSetup struct {
	ID           inputID
	Name         [80]byte
	FFEffectsMax uint32
}

type uinputKeyboard struct {
	fd int
	mu sync.Mutex
}

var (
	uinputOnce sync.Once
	uinputKB   *uinputKeyboard
	uinputErr  error
)

func uinputReady() bool {
	_, err := getUinputKeyboard()
	return err == nil
}

func pasteViaUinput() error {
	kb, err := getUinputKeyboard()
	if err != nil {
		return err
	}
	return kb.sendCtrlV()
}

func getUinputKeyboard() (*uinputKeyboard, error) {
	uinputOnce.Do(func() {
		uinputKB, uinputErr = openUinputKeyboard()
	})
	return uinputKB, uinputErr
}

func openUinputKeyboard() (*uinputKeyboard, error) {
	fd := -1
	var denied bool
	for _, path := range []string{"/dev/uinput", "/dev/input/uinput"} {
		nfd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
		if err == nil {
			fd = nfd
			break
		}
		if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) || errors.Is(err, os.ErrPermission) {
			denied = true
			continue
		}
		if !errors.Is(err, syscall.ENOENT) {
			return nil, fmt.Errorf("open uinput: %w", err)
		}
	}
	if fd < 0 {
		if denied {
			return nil, errUinputUnavailable
		}
		return nil, errUinputUnavailable
	}

	kb := &uinputKeyboard{fd: fd}
	if err := kb.configure(); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return kb, nil
}

func (kb *uinputKeyboard) configure() error {
	if err := ioctlSetInt(kb.fd, uiSetEvbit, evKey); err != nil {
		return fmt.Errorf("uinput UI_SET_EVBIT: %w", err)
	}
	for _, key := range []int{keyLeftCtrl, keyV} {
		if err := ioctlSetInt(kb.fd, uiSetKeybit, key); err != nil {
			return fmt.Errorf("uinput UI_SET_KEYBIT(%d): %w", key, err)
		}
	}
	var setup uinputSetup
	setup.ID.Bustype = 0x03
	copy(setup.Name[:], "LinBoard")
	if err := ioctlSetPtr(kb.fd, uiDevSetup, unsafe.Pointer(&setup)); err != nil {
		return fmt.Errorf("uinput UI_DEV_SETUP: %w", err)
	}
	if err := ioctlSetInt(kb.fd, uiDevCreate, 0); err != nil {
		return fmt.Errorf("uinput UI_DEV_CREATE: %w", err)
	}
	return nil
}

func (kb *uinputKeyboard) sendCtrlV() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	for _, step := range []struct {
		code  uint16
		value int32
	}{
		{keyLeftCtrl, 1},
		{keyV, 1},
		{keyV, 0},
		{keyLeftCtrl, 0},
	} {
		if err := kb.emitKey(step.code, step.value); err != nil {
			return err
		}
		time.Sleep(8 * time.Millisecond)
	}
	return nil
}

func (kb *uinputKeyboard) emitKey(code uint16, value int32) error {
	ev := inputEvent{Type: evKey, Code: code, Value: value}
	if err := kb.writeEvent(&ev); err != nil {
		return err
	}
	sync := inputEvent{Type: evSyn, Code: synReport, Value: 0}
	return kb.writeEvent(&sync)
}

func (kb *uinputKeyboard) writeEvent(ev *inputEvent) error {
	buf := (*[unsafe.Sizeof(inputEvent{})]byte)(unsafe.Pointer(ev))
	n, err := syscall.Write(kb.fd, buf[:])
	if err != nil {
		return err
	}
	if n != int(unsafe.Sizeof(inputEvent{})) {
		return fmt.Errorf("uinput: short write (%d bytes)", n)
	}
	return nil
}

func ioctlSetInt(fd int, req uintptr, value int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(value))
	if errno != 0 {
		return errno
	}
	return nil
}

func ioctlSetPtr(fd int, req uintptr, ptr unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(ptr))
	if errno != 0 {
		return errno
	}
	return nil
}
