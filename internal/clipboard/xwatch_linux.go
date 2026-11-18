//go:build linux

package clipboard

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <errno.h>
#include <stdint.h>
#include <string.h>
#include <sys/select.h>
#include <time.h>

static Display *watchDisplay;
static Window watchWindow;
static Atom atomClipboard, atomTimestamp, atomStampProp, atomUtf8, atomIncr;
static int watchState; // 0 unknown, 1 open, -1 unavailable

static int ensureWatch(void) {
	if (watchState != 0) {
		return watchState == 1;
	}
	watchDisplay = XOpenDisplay(NULL);
	if (!watchDisplay) {
		watchState = -1;
		return 0;
	}
	int screen = DefaultScreen(watchDisplay);
	watchWindow = XCreateSimpleWindow(watchDisplay, RootWindow(watchDisplay, screen),
		0, 0, 1, 1, 0, 0, 0);
	atomClipboard = XInternAtom(watchDisplay, "CLIPBOARD", False);
	atomTimestamp = XInternAtom(watchDisplay, "TIMESTAMP", False);
	atomStampProp = XInternAtom(watchDisplay, "LINBOARD_STAMP", False);
	atomUtf8 = XInternAtom(watchDisplay, "UTF8_STRING", False);
	atomIncr = XInternAtom(watchDisplay, "INCR", False);
	watchState = 1;
	return 1;
}

// waitForSelection returns 1 when the reply has a property, -1 when the owner
// rejects the target, and 0 on timeout.
static int waitForSelection(int timeoutMs) {
	int fd = ConnectionNumber(watchDisplay);
	struct timespec deadline;
	clock_gettime(CLOCK_MONOTONIC, &deadline);
	deadline.tv_sec += timeoutMs / 1000;
	deadline.tv_nsec += (long)(timeoutMs % 1000) * 1000000L;
	if (deadline.tv_nsec >= 1000000000L) {
		deadline.tv_sec++;
		deadline.tv_nsec -= 1000000000L;
	}

	for (;;) {
		while (XPending(watchDisplay)) {
			XEvent ev;
			XNextEvent(watchDisplay, &ev);
			if (ev.type == SelectionNotify &&
				ev.xselection.selection == atomClipboard &&
				ev.xselection.requestor == watchWindow) {
				return ev.xselection.property != None ? 1 : -1;
			}
		}

		struct timespec now;
		clock_gettime(CLOCK_MONOTONIC, &now);
		long leftMs = (deadline.tv_sec - now.tv_sec) * 1000L +
			(deadline.tv_nsec - now.tv_nsec) / 1000000L;
		if (leftMs <= 0) {
			return 0;
		}
		fd_set fds;
		FD_ZERO(&fds);
		FD_SET(fd, &fds);
		struct timeval tv;
		tv.tv_sec = leftMs / 1000;
		tv.tv_usec = (leftMs % 1000) * 1000;
		int rc = select(fd + 1, &fds, NULL, NULL, &tv);
		if (rc < 0 && errno != EINTR) {
			return 0;
		}
	}
}

// clipboardStampC reports the CLIPBOARD TIMESTAMP.
// Returns 0 when no X display is available, 2 on timeout, 1 and writes *out otherwise.
// An empty clipboard (no owner) is stamp 0 with a successful return.
static int clipboardStampC(unsigned long *out, int timeoutMs) {
	*out = 0;
	if (!ensureWatch()) {
		return 0;
	}
	Window owner = XGetSelectionOwner(watchDisplay, atomClipboard);
	if (!owner) {
		return 1;
	}
	XDeleteProperty(watchDisplay, watchWindow, atomStampProp);
	XConvertSelection(watchDisplay, atomClipboard, atomTimestamp, atomStampProp,
		watchWindow, CurrentTime);
	XFlush(watchDisplay);
	int reply = waitForSelection(timeoutMs);
	if (reply == 0) {
		return 2;
	}
	if (reply < 0) {
		*out = (unsigned long)owner;
		return 1;
	}
	Atom actualType;
	int actualFormat;
	unsigned long count = 0, remaining = 0;
	unsigned char *data = NULL;
	int status = XGetWindowProperty(watchDisplay, watchWindow, atomStampProp,
		0, 1, True, AnyPropertyType,
		&actualType, &actualFormat, &count, &remaining, &data);
	if (status != Success || !data || count < 1 || actualFormat != 32) {
		if (data) {
			XFree(data);
		}
		// Owner did not answer TIMESTAMP. The window id still changes when a
		// different client takes the selection.
		*out = (unsigned long)owner;
		return 1;
	}
	*out = ((unsigned long *)data)[0];
	XFree(data);
	if (*out == 0) {
		*out = (unsigned long)owner;
	}
	return 1;
}

// clipboardPeekC reads plain text without spawning a process.
// 0: no X display. 1: success (*outLen may be 0). 2: timeout.
// 3: the offer is INCR, too large, or not plain text — caller uses xclip.
// *hasImage is set on success when an image/* target is advertised.
static int clipboardPeekC(char *buf, int cap, int *outLen, int *hasImage, int timeoutMs) {
	*outLen = 0;
	*hasImage = 0;
	if (!ensureWatch() || cap < 1) {
		return 0;
	}
	Window owner = XGetSelectionOwner(watchDisplay, atomClipboard);
	if (!owner) {
		return 1;
	}

	Atom targets = XInternAtom(watchDisplay, "TARGETS", False);
	XDeleteProperty(watchDisplay, watchWindow, atomStampProp);
	XConvertSelection(watchDisplay, atomClipboard, targets, atomStampProp,
		watchWindow, CurrentTime);
	XFlush(watchDisplay);
	int reply = waitForSelection(timeoutMs);
	if (reply == 0) {
		return 2;
	}
	if (reply < 0) {
		return 1;
	}

	Atom actualType;
	int actualFormat;
	unsigned long count = 0, remaining = 0;
	unsigned char *data = NULL;
	int status = XGetWindowProperty(watchDisplay, watchWindow, atomStampProp,
		0, 64, True, AnyPropertyType,
		&actualType, &actualFormat, &count, &remaining, &data);
	if (status != Success || !data || actualFormat != 32) {
		if (data) {
			XFree(data);
		}
		return 2;
	}

	Atom want = None;
	int otherText = 0;
	Atom *atoms = (Atom *)data;
	unsigned long i;
	for (i = 0; i < count; i++) {
		char *name = XGetAtomName(watchDisplay, atoms[i]);
		if (!name) {
			continue;
		}
		if (strncmp(name, "image/", 6) == 0) {
			*hasImage = 1;
		}
		if (atoms[i] == atomUtf8 || strcmp(name, "UTF8_STRING") == 0) {
			want = atomUtf8;
		} else if (want != atomUtf8 && (strcmp(name, "text/plain") == 0 ||
			strcmp(name, "text/plain;charset=utf-8") == 0 ||
			atoms[i] == XA_STRING)) {
			want = atoms[i];
		} else if (strncmp(name, "text/", 5) == 0) {
			// HTML or a URI list, used only when no plain target exists.
			otherText = 1;
		}
		XFree(name);
	}
	XFree(data);
	if (want == None) {
		return otherText ? 3 : 1;
	}

	XDeleteProperty(watchDisplay, watchWindow, atomStampProp);
	XConvertSelection(watchDisplay, atomClipboard, want, atomStampProp,
		watchWindow, CurrentTime);
	XFlush(watchDisplay);
	reply = waitForSelection(timeoutMs);
	if (reply == 0) {
		return 2;
	}
	if (reply < 0) {
		return 1;
	}
	status = XGetWindowProperty(watchDisplay, watchWindow, atomStampProp,
		0, (long)(cap / 4) + 1, True, AnyPropertyType,
		&actualType, &actualFormat, &count, &remaining, &data);
	if (status != Success || !data) {
		if (data) {
			XFree(data);
		}
		return 2;
	}
	if (actualType == atomIncr || actualFormat != 8 || remaining > 0 || count > (unsigned long)cap) {
		XFree(data);
		return 3;
	}
	if (count > 0) {
		memcpy(buf, data, count);
	}
	*outLen = (int)count;
	XFree(data);
	return 1;
}
*/
import "C"
import (
	"context"
	"errors"
	"unsafe"
)

// errStampTimeout means the selection owner did not answer. The poll skips the
// read so a wedged owner cannot stack xclip processes on top of the wait.
var errStampTimeout = errors.New("clipboard timestamp timed out")

// clipboardStamp reads the X11 CLIPBOARD timestamp without spawning a process.
// watched is false when there is no X display; callers then poll as before.
func clipboardStamp(ctx context.Context) (uint64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, true, err
	}
	var stamp C.ulong
	switch C.clipboardStampC(&stamp, 200) {
	case 0:
		return 0, false, nil
	case 2:
		return 0, true, errStampTimeout
	default:
		return uint64(stamp), true, nil
	}
}

// peekClipboard reads the current plain-text offer on the open X connection.
// ok is false when the caller should use xclip instead (no display, timeout,
// INCR, or a target this peek does not decode).
func peekClipboard(ctx context.Context) (text string, hasImage bool, ok bool) {
	if err := ctx.Err(); err != nil {
		return "", false, false
	}
	buf := make([]byte, 256*1024)
	var n, image C.int
	switch C.clipboardPeekC((*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)), &n, &image, 200) {
	case 1:
		if n < 0 || int(n) > len(buf) {
			return "", false, false
		}
		return string(buf[:int(n)]), image != 0, true
	default:
		return "", false, false
	}
}
