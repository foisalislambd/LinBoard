package clipboard

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/foisal/linboard/internal/store"
)

func testMonitorStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", dir+"/config")
	t.Setenv("XDG_DATA_HOME", dir+"/data")
	s, err := store.Open(20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestAcceptTextSkipsBinaryAndOversize(t *testing.T) {
	m := NewMonitor(testMonitorStore(t))
	buf := captureLog(t)

	png := "\x89PNG\r\n\x1a\nnot-a-text-clip"
	m.acceptText(png, nil)
	m.acceptText(png, nil)
	m.acceptText("", store.ErrTooLarge)
	m.acceptText("", store.ErrTooLarge)

	n, err := m.store.Count()
	if err != nil || n != 0 {
		t.Fatalf("stored %d (%v)", n, err)
	}
	logs := buf.String()
	if strings.Count(logs, "not stored") != 2 {
		t.Fatalf("logs:\n%s", logs)
	}
	if !strings.Contains(logs, "exceeds") || !strings.Contains(logs, "binary") {
		t.Fatalf("logs:\n%s", logs)
	}
	clips, err := m.store.List("", 20)
	if err != nil || len(clips) != 0 {
		t.Fatal(clips, err)
	}
}

func TestMonitorCancelsStuckReader(t *testing.T) {
	m := NewMonitor(testMonitorStore(t))
	started := make(chan struct{})
	var once sync.Once
	m.capture = func(ctx context.Context, _ bool) (string, error, []byte, error) {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return "", ctx.Err(), nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reader did not start")
	}
	cancel()
	if !m.WaitTimeout(time.Second) {
		t.Fatal("monitor did not stop after cancel")
	}
}

func TestWaitTimeoutWhileReaderStuck(t *testing.T) {
	m := NewMonitor(testMonitorStore(t))
	release := make(chan struct{})
	m.capture = func(ctx context.Context, _ bool) (string, error, []byte, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return "", nil, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	if m.WaitTimeout(150 * time.Millisecond) {
		t.Fatal("WaitTimeout returned while the reader was stuck")
	}
	close(release)
	cancel()
	if !m.WaitTimeout(time.Second) {
		t.Fatal("monitor did not stop after the reader was released")
	}
}

func TestUnchangedStampDoesNotReread(t *testing.T) {
	var stamp atomic.Uint64
	stamp.Store(5)
	var calls atomic.Int32
	m := NewMonitor(testMonitorStore(t))
	m.stamp = func(context.Context) (uint64, bool, error) {
		return stamp.Load(), true, nil
	}
	m.capture = func(_ context.Context, wantImage bool) (string, error, []byte, error) {
		calls.Add(1)
		if !wantImage {
			t.Errorf("a watched change must read image targets in the same pass")
		}
		return "hello", nil, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	time.Sleep(700 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("captures = %d, want 1", got)
	}
	stamp.Store(6)
	time.Sleep(600 * time.Millisecond)
	if got := calls.Load(); got != 2 {
		t.Fatalf("captures after change = %d, want 2", got)
	}
}

func TestOutputLimitedKillsStuckCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := outputLimited(ctx, "sleep", []string{"30"}, 10*time.Second, 32)
	if time.Since(start) > 2*time.Second {
		t.Fatalf("stuck command was not killed (%v)", err)
	}
	if err == nil {
		t.Fatal("expected an error from the killed command")
	}
}

func TestOutputLimitedRejectsHugeStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := outputLimited(ctx, "sh", []string{"-c", "dd if=/dev/zero bs=1024 count=64 status=none"}, 3*time.Second, 128)
	if !errors.Is(err, store.ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
}
