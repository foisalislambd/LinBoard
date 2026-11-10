package relversion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	v, err := Parse("  v1.2.3\n")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1.2.3" || v.Tag() != "v1.2.3" {
		t.Fatalf("got %s %s", v.String(), v.Tag())
	}
	if _, err := Parse("1.2"); err == nil {
		t.Fatal("expected error for 1.2")
	}
	if _, err := Parse("1.2.3-rc1"); err == nil {
		t.Fatal("expected error for prerelease")
	}
	if _, err := Parse("01.2.3"); err == nil {
		t.Fatal("expected error for leading zero")
	}
}

func TestCompare(t *testing.T) {
	a, _ := Parse("1.10.0")
	b, _ := Parse("1.9.9")
	if a.Compare(b) <= 0 {
		t.Fatal("1.10.0 should be greater than 1.9.9")
	}
	if b.Compare(a) >= 0 {
		t.Fatal("1.9.9 should be less than 1.10.0")
	}
	same, _ := Parse("v1.10.0")
	if a.Compare(same) != 0 {
		t.Fatal("expected equal")
	}
}

func TestDecide(t *testing.T) {
	d, err := Decide("1.0.14", []string{"v1.0.13", "v1.0.9", "not-a-version"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Release || d.Version.Tag() != "v1.0.14" {
		t.Fatalf("want release v1.0.14, got %+v", d)
	}

	d, err = Decide("1.0.13", []string{"v1.0.13"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Release {
		t.Fatal("equal version must skip")
	}

	d, err = Decide("1.0.12", []string{"LinBoard v1.0.13", "v1.0.13"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Release {
		t.Fatal("lower version must skip")
	}

	d, err = Decide("1.0.0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Release {
		t.Fatal("first release should publish")
	}

	d, err = Decide("2.0.0", []string{"nightly", "latest"})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Release {
		t.Fatal("non-semver names should not block the first real release")
	}

	if _, err := Decide("nope", nil); err == nil {
		t.Fatal("invalid VERSION.md must fail")
	}
}

func TestCheckedInVersionFile(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	for {
		path := filepath.Join(dir, "VERSION.md")
		raw, err = os.ReadFile(path)
		if err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("VERSION.md not found")
		}
		dir = parent
	}
	text := strings.TrimSpace(string(raw))
	if strings.Contains(text, "\n") {
		t.Fatalf("VERSION.md must be a single version, got %q", text)
	}
	if _, err := Parse(text); err != nil {
		t.Fatal(err)
	}
}
