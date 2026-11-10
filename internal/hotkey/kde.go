package hotkey

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/foisal/linboard/internal/atomicfile"
	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
	"github.com/godbus/dbus/v5"
)

type kdeBackend struct{}

func (b *kdeBackend) start(_ func()) error {
	exe, err := ExecutableForShortcut()
	if err != nil {
		return err
	}
	if err := setupKDEHotkey(exe); err != nil {
		return err
	}
	log.Printf("hotkey registered (KDE): %s → linboard toggle", config.HotkeyLabel)
	return nil
}

func (b *kdeBackend) stop() {}

func setupKDEHotkey(exe string) error {
	if !platform.IsKDE() {
		return skip("not KDE")
	}
	path := kdeHotkeysPath()
	if path == "" {
		return fmt.Errorf("home directory not found")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	updated := UpdateKHotkeys(readFile(path), exe+" toggle")
	if err := atomicfile.Write(path, []byte(updated), 0o644); err != nil {
		return err
	}
	reloadKHotkeys()
	return nil
}

func kdeHotkeysPath() string {
	base, err := config.ConfigHome()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "khotkeysrc")
}

type iniSection struct {
	name  string
	lines []string
}

var (
	dataIndexRe = regexp.MustCompile(`^Data_(\d+)$`)
	headerRe    = regexp.MustCompile(`^\[(.+)]\s*$`)
)

// UpdateKHotkeys inserts or refreshes a LinBoard shortcut in khotkeysrc.
// Other shortcuts, including commands that merely end in "toggle", are left unchanged.
// The previous writer used a section name KDE does not load and rewrote every
// Command= line ending in "toggle".
func UpdateKHotkeys(content, command string) string {
	sections := parseINI(content)
	kept := sections[:0]
	for _, s := range sections {
		if isBrokenLinboardSection(s.name) {
			continue
		}
		kept = append(kept, s)
	}
	sections = kept

	if n := findLinboardDataIndex(sections); n > 0 {
		sections = updateLinboardGroup(sections, n, command)
		if dataCountValue(sections) < n {
			sections = setDataCount(sections, n)
		}
		return renderINI(sections)
	}

	n := maxDataIndex(sections) + 1
	if n < 1 {
		n = 1
	}
	dc := dataCountValue(sections)
	if dc < n {
		dc = n
	}
	sections = setDataCount(sections, dc)
	sections = append(sections, newLinboardSections(n, command)...)
	return renderINI(sections)
}

func isBrokenLinboardSection(name string) bool {
	return strings.Contains(strings.ToLower(name), "linboard")
}

func findLinboardDataIndex(sections []iniSection) int {
	for _, s := range sections {
		n, ok := exactDataIndex(s.name)
		if !ok {
			continue
		}
		for _, ln := range s.lines {
			if strings.TrimSpace(ln) == "Name=LinBoard" {
				return n
			}
		}
	}
	actionRe := regexp.MustCompile(`^Data_(\d+)Actions\d*$`)
	for _, s := range sections {
		m := actionRe.FindStringSubmatch(s.name)
		if m == nil {
			continue
		}
		for _, ln := range s.lines {
			t := strings.TrimSpace(ln)
			lower := strings.ToLower(t)
			if (strings.HasPrefix(t, "CommandURL=") || strings.HasPrefix(t, "Command=")) &&
				strings.Contains(lower, "linboard") && strings.Contains(lower, "toggle") {
				n, _ := strconv.Atoi(m[1])
				return n
			}
		}
	}
	return 0
}

func exactDataIndex(name string) (int, bool) {
	m := dataIndexRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func maxDataIndex(sections []iniSection) int {
	max := 0
	for _, s := range sections {
		if n, ok := exactDataIndex(s.name); ok && n > max {
			max = n
		}
	}
	return max
}

func dataCountValue(sections []iniSection) int {
	for _, s := range sections {
		if s.name != "Data" {
			continue
		}
		for _, ln := range s.lines {
			t := strings.TrimSpace(ln)
			if strings.HasPrefix(t, "DataCount=") {
				n, err := strconv.Atoi(strings.TrimPrefix(t, "DataCount="))
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func setDataCount(sections []iniSection, n int) []iniSection {
	found := false
	val := strconv.Itoa(n)
	for i := range sections {
		if sections[i].name != "Data" {
			continue
		}
		found = true
		sections[i].lines = setOrReplaceKV(sections[i].lines, "DataCount", val)
	}
	if !found {
		sections = append([]iniSection{{
			name:  "Data",
			lines: []string{"DataCount=" + val},
		}}, sections...)
	}
	return sections
}

func updateLinboardGroup(sections []iniSection, n int, command string) []iniSection {
	root := fmt.Sprintf("Data_%d", n)
	action := fmt.Sprintf("Data_%dActions0", n)
	trigger := fmt.Sprintf("Data_%dTriggers0", n)
	haveAction := false
	haveTrigger := false
	for i := range sections {
		switch sections[i].name {
		case root:
			sections[i].lines = setOrReplaceKV(sections[i].lines, "Enabled", "true")
			sections[i].lines = setOrReplaceKV(sections[i].lines, "Name", "LinBoard")
			sections[i].lines = setOrReplaceKV(sections[i].lines, "Type", "SIMPLE_ACTION_DATA")
		case action:
			haveAction = true
			sections[i].lines = removeKV(sections[i].lines, "Command")
			sections[i].lines = setOrReplaceKV(sections[i].lines, "CommandURL", command)
			sections[i].lines = setOrReplaceKV(sections[i].lines, "Type", "COMMAND_URL")
		case trigger:
			haveTrigger = true
			sections[i].lines = setOrReplaceKV(sections[i].lines, "Key", "Meta+V")
			sections[i].lines = setOrReplaceKV(sections[i].lines, "Type", "SHORTCUT")
		}
	}
	if !haveAction || !haveTrigger {
		sections = append(sections, missingLinboardChildren(n, command, haveAction, haveTrigger)...)
	}
	return sections
}

func missingLinboardChildren(n int, command string, haveAction, haveTrigger bool) []iniSection {
	var out []iniSection
	if !haveAction {
		out = append(out,
			iniSection{name: fmt.Sprintf("Data_%dActions", n), lines: []string{"ActionsCount=1"}},
			iniSection{name: fmt.Sprintf("Data_%dActions0", n), lines: []string{
				"CommandURL=" + command,
				"Type=COMMAND_URL",
			}},
		)
	}
	if !haveTrigger {
		out = append(out,
			iniSection{name: fmt.Sprintf("Data_%dTriggers", n), lines: []string{"TriggersCount=1"}},
			iniSection{name: fmt.Sprintf("Data_%dTriggers0", n), lines: []string{
				"Key=Meta+V",
				"Type=SHORTCUT",
				"Uuid={linboard-toggle-0001}",
			}},
		)
	}
	return out
}

func newLinboardSections(n int, command string) []iniSection {
	id := fmt.Sprintf("Data_%d", n)
	return []iniSection{
		{name: id, lines: []string{
			"Comment=LinBoard clipboard history",
			"Enabled=true",
			"Name=LinBoard",
			"Type=SIMPLE_ACTION_DATA",
		}},
		{name: id + "Actions", lines: []string{"ActionsCount=1"}},
		{name: id + "Actions0", lines: []string{
			"CommandURL=" + command,
			"Type=COMMAND_URL",
		}},
		{name: id + "Conditions", lines: []string{"ConditionsCount=0"}},
		{name: id + "Triggers", lines: []string{"TriggersCount=1"}},
		{name: id + "Triggers0", lines: []string{
			"Key=Meta+V",
			"Type=SHORTCUT",
			"Uuid={linboard-toggle-0001}",
		}},
	}
}

func setOrReplaceKV(lines []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(lines)+1)
	replaced := false
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), prefix) {
			if !replaced {
				out = append(out, prefix+value)
				replaced = true
			}
			continue
		}
		out = append(out, ln)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func removeKV(lines []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), prefix) {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func parseINI(content string) []iniSection {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	// Drop the empty string produced by a trailing newline so we don't
	// invent a blank line at the end of the last section on every save.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	sections := []iniSection{{}}
	cur := 0
	for _, line := range lines {
		if m := headerRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			sections = append(sections, iniSection{name: m[1]})
			cur = len(sections) - 1
			continue
		}
		sections[cur].lines = append(sections[cur].lines, line)
	}
	return sections
}

func renderINI(sections []iniSection) string {
	var b strings.Builder
	for _, s := range sections {
		if s.name == "" && len(s.lines) == 0 {
			continue
		}
		if s.name != "" {
			b.WriteByte('[')
			b.WriteString(s.name)
			b.WriteString("]\n")
		}
		for _, ln := range s.lines {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func reloadKHotkeys() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for _, spec := range []struct {
		bus   string
		iface string
	}{
		{"org.kde.kded6", "org.kde.kded6"},
		{"org.kde.kded5", "org.kde.kded5"},
	} {
		obj := conn.Object(spec.bus, "/kded")
		_ = obj.Call(spec.iface+".reloadModule", 0, "khotkeys").Err
	}
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
