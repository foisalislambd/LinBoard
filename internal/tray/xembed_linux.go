//go:build linux

package tray

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <stdint.h>
#include <string.h>
#include <stdio.h>

static Display *trayDisplay;
static Window trayWin;

static int dock(void) {
	trayDisplay = XOpenDisplay(NULL);
	if (!trayDisplay) {
		return 1;
	}
	int screen = DefaultScreen(trayDisplay);
	char selname[64];
	snprintf(selname, sizeof selname, "_NET_SYSTEM_TRAY_S%d", screen);
	Atom selection = XInternAtom(trayDisplay, selname, False);
	Window owner = XGetSelectionOwner(trayDisplay, selection);
	if (!owner) {
		XCloseDisplay(trayDisplay);
		trayDisplay = NULL;
		return 2;
	}
	Window root = RootWindow(trayDisplay, screen);
	trayWin = XCreateSimpleWindow(trayDisplay, root, 0, 0, 24, 24, 0,
		BlackPixel(trayDisplay, screen), 0x2f6f4e);
	XStoreName(trayDisplay, trayWin, "LinBoard");
	XSelectInput(trayDisplay, trayWin, ButtonPressMask | ExposureMask | StructureNotifyMask);
	Atom xembed = XInternAtom(trayDisplay, "_XEMBED_INFO", False);
	uint32_t info[2];
	info[0] = 0; // version
	info[1] = 1; // mapped
	XChangeProperty(trayDisplay, trayWin, xembed, xembed, 32, PropModeReplace,
		(unsigned char *)info, 2);

	Atom opcode = XInternAtom(trayDisplay, "_NET_SYSTEM_TRAY_OPCODE", False);
	XClientMessageEvent ev;
	memset(&ev, 0, sizeof ev);
	ev.type = ClientMessage;
	ev.window = owner;
	ev.message_type = opcode;
	ev.format = 32;
	ev.data.l[0] = CurrentTime;
	ev.data.l[1] = 0; // SYSTEM_TRAY_REQUEST_DOCK
	ev.data.l[2] = (long)trayWin;
	XSendEvent(trayDisplay, owner, False, NoEventMask, (XEvent *)&ev);
	XMapWindow(trayDisplay, trayWin);
	XFlush(trayDisplay);
	return 0;
}

static int pollClick(void) {
	if (!trayDisplay) {
		return 0;
	}
	int clicked = 0;
	while (XPending(trayDisplay)) {
		XEvent ev;
		XNextEvent(trayDisplay, &ev);
		if (ev.type == ButtonPress) {
			clicked = 1;
		}
	}
	return clicked;
}

static void shutdownTray(void) {
	if (!trayDisplay) {
		return;
	}
	if (trayWin) {
		XDestroyWindow(trayDisplay, trayWin);
	}
	XCloseDisplay(trayDisplay);
	trayDisplay = NULL;
	trayWin = 0;
}

static int trayPresent(void) {
	Display *d = XOpenDisplay(NULL);
	if (!d) {
		return 0;
	}
	int screen = DefaultScreen(d);
	char selname[64];
	snprintf(selname, sizeof selname, "_NET_SYSTEM_TRAY_S%d", screen);
	Atom selection = XInternAtom(d, selname, False);
	Window owner = XGetSelectionOwner(d, selection);
	XCloseDisplay(d);
	return owner != None;
}
*/
import "C"

import (
	"errors"
	"os"
	"sync"
	"time"
)

func xembedTrayPresent() bool {
	if os.Getenv("DISPLAY") == "" {
		return false
	}
	return C.trayPresent() != 0
}

func startXEmbed(onClick func()) (func(), error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, errors.New("no DISPLAY")
	}
	ready := make(chan error, 1)
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		var err error
		switch C.dock() {
		case 0:
		case 1:
			err = errors.New("cannot open X display")
		default:
			err = errors.New("no XEmbed system tray")
		}
		ready <- err
		if err != nil {
			return
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		defer C.shutdownTray()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if C.pollClick() != 0 && onClick != nil {
					go onClick()
				}
			}
		}
	}()
	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
	case <-time.After(time.Second):
		once.Do(func() { close(stop) })
		return nil, errors.New("XEmbed tray timed out")
	}
	return func() { once.Do(func() { close(stop) }) }, nil
}
