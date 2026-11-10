//go:build linux

package platform

import "testing"

func TestParseGnomeEval(t *testing.T) {
	ok, val := parseGnomeEval("(true, '42')")
	if !ok || val != "42" {
		t.Fatalf("ok=%v val=%q", ok, val)
	}
	ok, val = parseGnomeEval(`(true, 'it\'s')`)
	if !ok || val != "it's" {
		t.Fatalf("ok=%v val=%q", ok, val)
	}
	if ok, _ := parseGnomeEval("(false, '')"); ok {
		t.Fatal("false should not parse")
	}
	if ok, _ := parseGnomeEval("nope"); ok {
		t.Fatal("garbage should not parse")
	}
}
