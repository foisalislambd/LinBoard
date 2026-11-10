package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "xdg-data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "xdg-state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(dir, "xdg-runtime"))
	return dir
}

func testStore(t *testing.T, max int) *Store {
	t.Helper()
	isolateEnv(t)

	s, err := Open(max)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAddTextDedupAndPrune(t *testing.T) {
	s := testStore(t, 3)

	c1, err := s.AddText("hello")
	if err != nil || c1 == nil {
		t.Fatalf("add hello: %v %v", c1, err)
	}
	if c1.ID != 0 {
		t.Fatalf("first id = %d, want 0", c1.ID)
	}

	c2, err := s.AddText("world")
	if err != nil || c2 == nil {
		t.Fatal(err)
	}
	if c2.ID == c1.ID {
		t.Fatal("expected unique ids")
	}

	// Re-copy bumps created_at but keeps same id.
	time.Sleep(5 * time.Millisecond)
	c1b, err := s.AddText("hello")
	if err != nil || c1b == nil {
		t.Fatal(err)
	}
	if c1b.ID != c1.ID {
		t.Fatalf("dedup id changed: %d -> %d", c1.ID, c1b.ID)
	}
	if !c1b.CreatedAt.After(c1.CreatedAt) {
		t.Fatal("dedup should refresh created_at")
	}

	_, _ = s.AddText("a")
	_, _ = s.AddText("b")
	_, _ = s.AddText("c")

	n, _ := s.Count()
	if n > 3 {
		t.Fatalf("prune kept %d clips, want <= 3", n)
	}
}

func TestPinnedSurvivesPrune(t *testing.T) {
	s := testStore(t, 2)

	c, _ := s.AddText("pinned")
	_ = s.TogglePin(c.ID)
	_, _ = s.AddText("x")
	_, _ = s.AddText("y")
	_, _ = s.AddText("z")

	got, err := s.GetByID(c.ID)
	if err != nil || got == nil {
		t.Fatal("pinned clip missing after prune")
	}
}

func TestPersistReload(t *testing.T) {
	isolateEnv(t)

	s1, err := Open(50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.AddText("persist me"); err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()

	s2, err := Open(50)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()

	clips, err := s2.List("", 10)
	if err != nil || len(clips) != 1 || clips[0].Content != "persist me" {
		t.Fatalf("reload failed: %v %+v", err, clips)
	}
	if s2.nextID < 1 {
		t.Fatalf("nextID not restored: %d", s2.nextID)
	}

	path, _ := DataFilePath()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("clips.json missing at %s: %v", path, err)
	}
	if filepath.Base(path) != "clips.json" {
		t.Fatal("unexpected data path")
	}
}

func TestPreviewTruncatesUTF8(t *testing.T) {
	s := testStore(t, 10)
	// 130 snowmen — byte-slicing would panic or split a rune.
	long := strings.Repeat("☃", 130)
	c, err := s.AddText(long)
	if err != nil || c == nil {
		t.Fatal(err)
	}
	if strings.Contains(c.Preview, "\ufffd") {
		t.Fatalf("preview contains replacement char: %q", c.Preview)
	}
	if !strings.HasSuffix(c.Preview, "…") {
		t.Fatalf("preview should be truncated: %q", c.Preview)
	}
}

func TestCorruptJSONRecovered(t *testing.T) {
	isolateEnv(t)

	s1, err := Open(50)
	if err != nil {
		t.Fatal(err)
	}
	path, err := DataFilePath()
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Close()
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(50)
	if err != nil {
		t.Fatalf("corrupt json should not prevent startup: %v", err)
	}
	defer func() { _ = s2.Close() }()
	n, _ := s2.Count()
	if n != 0 {
		t.Fatalf("expected empty history after corrupt recover, got %d", n)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("corrupt backup missing: %v", err)
	}
}

func TestPruneOnOpenHonorsMax(t *testing.T) {
	isolateEnv(t)

	s1, err := Open(50)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := s1.AddText(fmt.Sprintf("item-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	_ = s1.Close()

	s2, err := Open(3)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	n, _ := s2.Count()
	if n != 3 {
		t.Fatalf("open prune kept %d, want 3", n)
	}
}

func TestSearchClearAndDelete(t *testing.T) {
	s := testStore(t, 20)
	pinned, err := s.AddText("Keep Me")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TogglePin(pinned.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddText("other value"); err != nil {
		t.Fatal(err)
	}
	found, err := s.List("KEEP", 10)
	if err != nil || len(found) != 1 || found[0].ID != pinned.ID {
		t.Fatalf("search: %+v %v", found, err)
	}
	if err := s.ClearUnpinned(); err != nil {
		t.Fatal(err)
	}
	n, _ := s.Count()
	if n != 1 {
		t.Fatalf("clear kept %d", n)
	}
	if err := s.Delete(pinned.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ = s.Count(); n != 0 {
		t.Fatalf("delete left %d", n)
	}
	if err := s.Delete(pinned.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsOversizedTextAndImage(t *testing.T) {
	s := testStore(t, 10)
	huge := strings.Repeat("a", MaxTextBytes+1)
	if _, err := s.AddText(huge); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("text limit: %v", err)
	}
	img := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, MaxImageBytes)...)
	if _, err := s.AddImage(img); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("image limit: %v", err)
	}
	if _, err := s.AddImage([]byte("not a png")); err == nil {
		t.Fatal("expected non-png rejection")
	}
}

func TestImageRoundTripAndUnsafePath(t *testing.T) {
	dir := isolateEnv(t)
	s, err := Open(10)
	if err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4}
	clip, err := s.AddImage(png)
	if err != nil || clip == nil {
		t.Fatal(err)
	}
	got, err := s.ReadImage(clip)
	if err != nil || !bytes.Equal(got, png) {
		t.Fatalf("read image: %v %d", err, len(got))
	}
	if !s.IsLatestImage(png) {
		t.Fatal("expected latest image")
	}
	_ = s.Close()

	sentinel := filepath.Join(dir, "do-not-delete")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := DataFilePath()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"next_id":2,"clips":[{"id":1,"content_type":"image","image_path":"` + sentinel + `","preview":"[Image]","created_at":"2020-01-01T00:00:00Z","hash":"x"}]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(10)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	loaded, _ := s2.List("", 10)
	if len(loaded) != 1 || loaded[0].ImagePath != "" {
		t.Fatalf("unsafe path not cleared: %+v", loaded)
	}
	if err := s2.Delete(1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel removed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("clips.json mode %o", info.Mode().Perm())
	}
}

func TestStartupDoesNotBumpLatest(t *testing.T) {
	s := testStore(t, 10)
	first, err := s.AddText("same")
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsLatestText("same") {
		t.Fatal("expected latest")
	}
	if s.IsLatestText("other") {
		t.Fatal("other should not be latest")
	}
	if _, err := s.AddText("second"); err != nil {
		t.Fatal(err)
	}
	if s.IsLatestText("same") {
		t.Fatal("same is no longer latest")
	}
	got, err := s.GetByID(first.ID)
	if err != nil || got == nil || got.Content != "same" {
		t.Fatal(got)
	}
}
