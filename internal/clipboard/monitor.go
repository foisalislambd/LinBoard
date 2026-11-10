package clipboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/foisal/linboard/internal/platform"
	"github.com/foisal/linboard/internal/store"
)

const (
	textPollInterval  = 400 * time.Millisecond
	imagePollInterval = 1500 * time.Millisecond
)

type Monitor struct {
	store     *store.Store
	mu        sync.Mutex
	lastText  string
	lastImg   string
	lastStamp uint64
	haveStamp bool
	onChange  func()
	wg        sync.WaitGroup
	// capture overrides clipboard reads in tests. nil uses the system clipboard.
	capture func(ctx context.Context, wantImage bool) (string, error, []byte, error)
	// stamp overrides selection-change detection in tests.
	// The bool is false when no watcher is available (always re-read).
	// A non-nil error while watched means this tick should not also read.
	stamp func(ctx context.Context) (uint64, bool, error)
}

const (
	skipOversizeText = "skip:oversize-text"
	skipBinaryText   = "skip:binary-text"
	skipOversizeImg  = "skip:oversize-image"
)

var activeMonitor atomic.Pointer[Monitor]
var watchSuppress atomic.Int32

func NewMonitor(s *store.Store) *Monitor {
	return &Monitor{store: s}
}

func (m *Monitor) OnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Monitor) Start(ctx context.Context) {
	if m.capture == nil && ReadToolName() == "none" {
		log.Printf("clipboard monitor: no read backend available")
		return
	}
	if m.capture == nil && platform.UsePortalHotkey() && !haveCommand("xclip") {
		log.Printf("clipboard monitor: image history needs xclip (XWayland); text via Fyne core")
	}

	activeMonitor.Store(m)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.prime(ctx)
		m.poll(ctx)
	}()
}

// Wait blocks until the poll loop started by Start has returned.
func (m *Monitor) Wait() {
	m.wg.Wait()
}

// WaitTimeout reports whether the poll loop stopped before d elapsed.
// Shutdown uses this so a stuck clipboard read cannot hold the process open.
func (m *Monitor) WaitTimeout(d time.Duration) bool {
	if m == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		m.Wait()
		close(done)
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// prime records the current clipboard when it is already the newest history
// entry, so launching LinBoard does not bump that clip. Anything else is left
// for the poll loop to record.
func (m *Monitor) prime(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	stamp, watched, skip := m.beginRead(ctx)
	if skip {
		return
	}
	text, textErr, img, imgErr := m.captureNow(ctx, true)
	if ctx.Err() != nil {
		return
	}
	if textErr == nil && text != "" {
		text, textErr = sanitizeText(text)
	}
	if textErr == nil && text != "" && m.store.IsLatestText(text) {
		m.setLastText(textKey(text))
	}
	if imgErr == nil && len(img) > 0 && store.IsPNG(img) && m.store.IsLatestImage(img) {
		m.setLastImg(imageFingerprint(img))
	}
	m.finishRead(stamp, watched)
}

func (m *Monitor) poll(ctx context.Context) {
	ticker := time.NewTicker(textPollInterval)
	defer ticker.Stop()
	nextImage := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if watchSuppress.Load() > 0 || ctx.Err() != nil {
				continue
			}
			wantImage := !time.Now().Before(nextImage)
			if wantImage {
				nextImage = time.Now().Add(imagePollInterval)
			}
			m.pollOnce(ctx, wantImage)
		}
	}
}

func (m *Monitor) pollOnce(ctx context.Context, wantImage bool) {
	stamp, watched, skip := m.beginRead(ctx)
	if skip {
		return
	}
	// A selection watcher already told us the clipboard changed, so read text
	// and image together. The image interval only applies when we have to poll.
	if watched {
		wantImage = true
	}
	text, textErr, img, imgErr := m.captureNow(ctx, wantImage)
	if ctx.Err() != nil {
		return
	}
	if watchSuppress.Load() > 0 {
		return
	}
	m.acceptText(text, textErr)
	if wantImage {
		m.acceptImage(img, imgErr)
	}
	m.finishRead(stamp, watched)
}

// beginRead reports the current selection stamp. skip is true when the stamp
// is unchanged or the watcher timed out, so the poll must not shell out.
func (m *Monitor) beginRead(ctx context.Context) (stamp uint64, watched bool, skip bool) {
	if ctx.Err() != nil {
		return 0, false, true
	}
	fn := m.stamp
	if fn == nil {
		fn = clipboardStamp
	}
	stamp, watched, err := fn(ctx)
	if !watched {
		return 0, false, false
	}
	if err != nil {
		return 0, true, true
	}
	m.mu.Lock()
	same := m.haveStamp && m.lastStamp == stamp
	m.mu.Unlock()
	return stamp, true, same
}

func (m *Monitor) finishRead(stamp uint64, watched bool) {
	if !watched {
		return
	}
	m.mu.Lock()
	m.haveStamp = true
	m.lastStamp = stamp
	m.mu.Unlock()
}

func (m *Monitor) acceptText(text string, err error) {
	if err == nil {
		text, err = sanitizeText(text)
	}
	if err != nil {
		switch {
		case errors.Is(err, store.ErrTooLarge):
			m.skipText(skipOversizeText, "clipboard text exceeds %d bytes; not stored", store.MaxTextBytes)
		case errors.Is(err, errBinaryClip):
			m.skipText(skipBinaryText, "clipboard text is binary or image data; not stored")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return
		default:
			return
		}
		return
	}
	if text == "" {
		return
	}

	key := textKey(text)
	m.mu.Lock()
	seen := key == m.lastText
	m.mu.Unlock()
	if seen {
		return
	}

	if _, err := m.store.AddText(text); err != nil {
		if errors.Is(err, store.ErrTooLarge) {
			m.skipText(skipOversizeText, "clipboard text exceeds %d bytes; not stored", store.MaxTextBytes)
			return
		}
		log.Printf("save text clip: %v", err)
		return
	}
	m.setLastText(key)
	m.notify()
}

func (m *Monitor) acceptImage(data []byte, err error) {
	if err != nil {
		if errors.Is(err, store.ErrTooLarge) {
			m.skipImage(skipOversizeImg, "clipboard image exceeds %d bytes; not stored", store.MaxImageBytes)
		}
		return
	}
	if len(data) == 0 {
		return
	}
	if !store.IsPNG(data) {
		return
	}

	key := imageFingerprint(data)
	m.mu.Lock()
	seen := key == m.lastImg
	m.mu.Unlock()
	if seen {
		return
	}

	if _, err := m.store.AddImage(data); err != nil {
		if errors.Is(err, store.ErrTooLarge) {
			m.skipImage(skipOversizeImg, "clipboard image exceeds %d bytes; not stored", store.MaxImageBytes)
			return
		}
		log.Printf("save image clip: %v", err)
		return
	}
	m.setLastImg(key)
	m.notify()
}

func (m *Monitor) skipText(key, format string, args ...any) {
	m.mu.Lock()
	seen := m.lastText == key
	m.lastText = key
	m.mu.Unlock()
	if !seen {
		log.Printf(format, args...)
	}
}

func (m *Monitor) skipImage(key, format string, args ...any) {
	m.mu.Lock()
	seen := m.lastImg == key
	m.lastImg = key
	m.mu.Unlock()
	if !seen {
		log.Printf(format, args...)
	}
}

func (m *Monitor) captureNow(ctx context.Context, wantImage bool) (string, error, []byte, error) {
	if m.capture != nil {
		return m.capture(ctx, wantImage)
	}
	return defaultCapture(ctx, wantImage)
}

func defaultCapture(ctx context.Context, wantImage bool) (string, error, []byte, error) {
	if err := ctx.Err(); err != nil {
		return "", err, nil, nil
	}
	// One X11 selection client at a time, and TARGETS is read once per poll so
	// text and image requests cannot race or pull the wrong target.
	if !platform.UsePortalHotkey() && haveCommand("xclip") {
		return captureXClip(ctx, wantImage)
	}
	if !platform.UsePortalHotkey() && haveCommand("xsel") {
		text, err := readTextXSel(ctx)
		if err != nil {
			return "", nil, nil, nil
		}
		text, err = sanitizeText(text)
		return text, err, nil, nil
	}
	text, textErr := readTextContext(ctx)
	if !wantImage || !haveCommand("xclip") || ctx.Err() != nil {
		return text, textErr, nil, nil
	}
	if !clipboardHasImage(ctx) {
		return text, textErr, nil, nil
	}
	img, imgErr := readImageContext(ctx)
	return text, textErr, img, imgErr
}

func captureXClip(ctx context.Context, wantImage bool) (string, error, []byte, error) {
	raw, err := readTargets(ctx)
	if err != nil || ctx.Err() != nil {
		if ctx.Err() != nil {
			return "", ctx.Err(), nil, nil
		}
		return "", nil, nil, nil
	}
	plan := planReads(raw)
	var text string
	var textErr error
	if plan.ReadText {
		text, textErr = readExplicitText(ctx)
		if textErr == nil {
			text, textErr = sanitizeText(text)
		}
	}
	if !wantImage || !plan.ReadImage || ctx.Err() != nil {
		return text, textErr, nil, nil
	}
	img, imgErr := readImageXClip(ctx)
	return text, textErr, img, imgErr
}

func (m *Monitor) notify() {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (m *Monitor) setLastText(key string) {
	m.mu.Lock()
	m.lastText = key
	m.mu.Unlock()
}

func (m *Monitor) setLastImg(key string) {
	m.mu.Lock()
	m.lastImg = key
	m.mu.Unlock()
}

// HoldWatch pauses clipboard ingest (pair with ReleaseWatch).
func HoldWatch() { watchSuppress.Add(1) }

// ReleaseWatch resumes clipboard ingest after HoldWatch.
func ReleaseWatch() { watchSuppress.Add(-1) }

// CopyClip puts clip content on the system clipboard without pasting.
func CopyClip(st *store.Store, clip *store.Clip) error {
	if clip == nil {
		return errors.New("no clip")
	}
	watchSuppress.Add(1)
	// Hold ingest until the compositor has the new clipboard (and paste can finish).
	defer time.AfterFunc(500*time.Millisecond, func() { watchSuppress.Add(-1) })

	switch clip.ContentType {
	case store.TypeImage:
		if st == nil {
			return errors.New("no store")
		}
		data, err := st.ReadImage(clip)
		if err != nil {
			return err
		}
		if err := WriteImage(data); err != nil {
			return err
		}
		noteImageSeen(data)
		return nil
	default:
		if err := WriteText(clip.Content); err != nil {
			return err
		}
		noteTextSeen(clip.Content)
		return nil
	}
}

func noteTextSeen(text string) {
	m := activeMonitor.Load()
	if m == nil {
		return
	}
	m.setLastText(textKey(text))
}

func noteImageSeen(data []byte) {
	m := activeMonitor.Load()
	if m == nil {
		return
	}
	m.setLastImg(imageFingerprint(data))
}

func textKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}

func imageFingerprint(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:16])
}
