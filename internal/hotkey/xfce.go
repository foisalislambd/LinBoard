package hotkey

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

const xfceSuperV = "<Super>v"

type xfceBackend struct{}

func (b *xfceBackend) start(_ func()) error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	if err := setupXFCEHotkey(exe); err != nil {
		return err
	}
	log.Printf("hotkey registered (XFCE): %s → linboard toggle", config.HotkeyLabel)
	return nil
}

func (b *xfceBackend) stop() {}

type xfceChannel struct {
	XMLName  xml.Name       `xml:"channel"`
	Name     string         `xml:"name,attr"`
	Version  string         `xml:"version,attr"`
	Property []xfceProperty `xml:"property"`
}

type xfceProperty struct {
	Name     string         `xml:"name,attr"`
	Type     string         `xml:"type,attr"`
	Value    string         `xml:"value,attr,omitempty"`
	Property []xfceProperty `xml:"property"`
}

func setupXFCEHotkey(exe string) error {
	if !platform.IsXFCE() {
		return skip("not XFCE")
	}

	cmd := exe + " toggle"
	// Prefer xfconf-query so a running XFCE session picks up the binding immediately.
	if hasBin("xfconf-query") {
		prop := "/commands/custom/" + xfceSuperV
		_ = exec.Command("xfconf-query", "--channel", "xfce4-keyboard-shortcuts", "--property", prop, "--reset").Run()
		if err := run("xfconf-query",
			"--channel", "xfce4-keyboard-shortcuts",
			"--property", prop,
			"--create", "--type", "string", "--set", cmd,
		); err != nil {
			return err
		}
		return nil
	}

	return writeXFCEShortcutXML(exe)
}

func writeXFCEShortcutXML(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".config", "xfce4", "xfconf", "xfce-perchannel-xml", "xfce4-keyboard-shortcuts.xml")

	ch := xfceChannel{Name: "xfce4-keyboard-shortcuts", Version: "1.0"}
	if data, err := os.ReadFile(path); err == nil {
		_ = xml.Unmarshal(data, &ch)
	}
	if ch.Name == "" {
		ch.Name = "xfce4-keyboard-shortcuts"
		ch.Version = "1.0"
	}

	commands := findOrCreateProperty(&ch.Property, "commands", "empty")
	custom := findOrCreateProperty(&commands.Property, "custom", "empty")

	// Remove legacy wrong layout (property name "LinBoard" with nested default key).
	custom.Property = filterOutProperty(custom.Property, "LinBoard")

	cmd := exe + " toggle"
	if existing := findProperty(&custom.Property, xfceSuperV); existing != nil {
		existing.Type = "string"
		existing.Value = cmd
		existing.Property = nil
	} else {
		custom.Property = append(custom.Property, xfceProperty{
			Name:  xfceSuperV,
			Type:  "string",
			Value: cmd,
		})
	}

	out, err := xml.MarshalIndent(ch, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), out...), 0o644)
}

func findProperty(props *[]xfceProperty, name string) *xfceProperty {
	for i := range *props {
		if (*props)[i].Name == name {
			return &(*props)[i]
		}
	}
	return nil
}

func findOrCreateProperty(props *[]xfceProperty, name, typ string) *xfceProperty {
	if p := findProperty(props, name); p != nil {
		return p
	}
	*props = append(*props, xfceProperty{Name: name, Type: typ})
	return &(*props)[len(*props)-1]
}

func filterOutProperty(props []xfceProperty, name string) []xfceProperty {
	out := props[:0]
	for _, p := range props {
		if p.Name == name {
			continue
		}
		out = append(out, p)
	}
	return out
}

func verifyXFCEShortcut(r VerifyReport, exe string) VerifyReport {
	r.OK = append(r.OK, "desktop: XFCE")
	want := exe + " toggle"
	if hasBin("xfconf-query") {
		out, err := exec.Command("xfconf-query",
			"--channel", "xfce4-keyboard-shortcuts",
			"--property", "/commands/custom/"+xfceSuperV,
		).Output()
		if err != nil {
			r.Fail = append(r.Fail, "XFCE Super+V shortcut not registered")
			return r
		}
		got := strings.TrimSpace(string(out))
		if got != want {
			r.Warn = append(r.Warn, fmt.Sprintf("XFCE command is %q (want %q)", got, want))
		} else {
			r.OK = append(r.OK, "XFCE binding: Super+V")
		}
		return r
	}

	home, err := os.UserHomeDir()
	if err != nil {
		r.Warn = append(r.Warn, "could not verify XFCE shortcut (no home dir)")
		return r
	}
	path := filepath.Join(home, ".config", "xfce4", "xfconf", "xfce-perchannel-xml", "xfce4-keyboard-shortcuts.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		r.Fail = append(r.Fail, "XFCE keyboard shortcuts file missing")
		return r
	}
	if !strings.Contains(string(data), want) || !strings.Contains(string(data), xfceSuperV) {
		r.Fail = append(r.Fail, "XFCE Super+V shortcut not in config")
		return r
	}
	r.OK = append(r.OK, "XFCE binding present in config")
	return r
}
