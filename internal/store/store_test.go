package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T, max int) *Store {
	t.Helper()
	dir := t.TempDir()
	// config.DataDir uses os.UserHomeDir — HOME on Unix, USERPROFILE on Windows.
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

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
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

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
	defer s2.Close()

	clips, err := s2.List("", 10)
	if err != nil || len(clips) != 1 || clips[0].Content != "persist me" {
		t.Fatalf("reload failed: %v %+v", err, clips)
	}
	if s2.fileStore.nextID < 1 {
		t.Fatalf("nextID not restored: %d", s2.fileStore.nextID)
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
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

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
	defer s2.Close()
	n, _ := s2.Count()
	if n != 0 {
		t.Fatalf("expected empty history after corrupt recover, got %d", n)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("corrupt backup missing: %v", err)
	}
}

func TestPruneOnOpenHonorsMax(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

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
	defer s2.Close()
	n, _ := s2.Count()
	if n != 3 {
		t.Fatalf("open prune kept %d, want 3", n)
	}
}
