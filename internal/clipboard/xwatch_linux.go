//go:build linux

package clipboard

/*
#cgo pkg-config: x11
#include <X11/Xlib.h>
#include <errno.h>
#include <stdint.h>
#include <sys/select.h>
#include <time.h>

static Display *watchDisplay;
static Window watchWindow;
static Atom atomClipboard, atomTimestamp, atomStampProp;
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
*/
import "C"
import (
	"context"
	"errors"
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
