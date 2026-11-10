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
	store    *store.Store
	mu       sync.Mutex
	lastText string
	lastImg  string
	onChange func()
	wg       sync.WaitGroup
}

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
	if ReadToolName() == "none" {
		log.Printf("clipboard monitor: no read backend available")
		return
	}

	activeMonitor.Store(m)
	m.prime(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.pollText(ctx)
	}()
	if haveCommand("xclip") {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.pollImage(ctx)
		}()
	} else if platform.UsePortalHotkey() {
		log.Printf("clipboard monitor: image history needs xclip (XWayland); text via Fyne core")
	}
}

// Wait blocks until the poll loops started by Start have returned.
func (m *Monitor) Wait() {
	m.wg.Wait()
}

// prime records the current clipboard when it is already the newest history
// entry, so launching LinBoard does not bump that clip. Anything else is left
// for the poll loop to record.
func (m *Monitor) prime(ctx context.Context) {
	text, err := readTextContext(ctx)
	if err == nil && text != "" && m.store.IsLatestText(text) {
		m.setLastText(textKey(text))
	}
	if !haveCommand("xclip") || !clipboardHasImage(ctx) {
		return
	}
	data, err := readImageContext(ctx)
	if err != nil || len(data) == 0 {
		return
	}
	if m.store.IsLatestImage(data) {
		m.setLastImg(imageFingerprint(data))
	}
}

func (m *Monitor) pollText(ctx context.Context) {
	ticker := time.NewTicker(textPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if watchSuppress.Load() > 0 {
				continue
			}

			text, err := readTextContext(ctx)
			if err != nil || text == "" {
				continue
			}
			if watchSuppress.Load() > 0 {
				continue
			}

			key := textKey(text)
			m.mu.Lock()
			seen := key == m.lastText
			m.mu.Unlock()
			if seen {
				continue
			}

			if _, err := m.store.AddText(text); err != nil {
				if errors.Is(err, store.ErrTooLarge) {
					m.setLastText(key)
					log.Printf("clipboard text exceeds %d bytes; not stored", store.MaxTextBytes)
				} else {
					log.Printf("save text clip: %v", err)
				}
				continue
			}
			m.setLastText(key)
			m.notify()
		}
	}
}

func (m *Monitor) pollImage(ctx context.Context) {
	ticker := time.NewTicker(imagePollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if watchSuppress.Load() > 0 {
				continue
			}
			if !clipboardHasImage(ctx) {
				continue
			}

			data, err := readImageContext(ctx)
			if err != nil || len(data) == 0 {
				continue
			}
			if watchSuppress.Load() > 0 {
				continue
			}

			key := imageFingerprint(data)
			m.mu.Lock()
			seen := key == m.lastImg
			m.mu.Unlock()
			if seen {
				continue
			}

			if _, err := m.store.AddImage(data); err != nil {
				if errors.Is(err, store.ErrTooLarge) || !store.IsPNG(data) {
					m.setLastImg(key)
					log.Printf("clipboard image not stored: %v", err)
				} else {
					log.Printf("save image clip: %v", err)
				}
				continue
			}
			m.setLastImg(key)
			m.notify()
		}
	}
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
