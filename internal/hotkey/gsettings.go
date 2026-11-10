package hotkey

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode"
)

const gnomeMediaKeys = "org.gnome.settings-daemon.plugins.media-keys"

func gsettingsGet(schema, key string) (string, error) {
	out, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gsettingsSet(schema, key, value string) error {
	cmd := exec.Command("gsettings", "set", schema, key, value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings set %s %s: %w (%s)", schema, key, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// gvariantString quotes s as a GVariant string (double quotes, escaped).
// Bare values break on bindings such as <Super>v, which GVariant parses as a type.
func gvariantString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n', '\r':
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func gsettingsSetString(schema, key, value string) error {
	return gsettingsSet(schema, key, gvariantString(value))
}

func gsettingsListPaths() ([]string, error) {
	out, err := exec.Command("gsettings", "get", gnomeMediaKeys, "custom-keybindings").Output()
	if err != nil {
		return nil, err
	}
	return parseGSettingsStringArray(strings.TrimSpace(string(out))), nil
}

func gsettingsSetPaths(paths []string) error {
	return gsettingsSetArray(gnomeMediaKeys, "custom-keybindings", paths)
}

func gsettingsGetArray(schema, key string) ([]string, error) {
	out, err := gsettingsGet(schema, key)
	if err != nil {
		return nil, err
	}
	return parseGSettingsStringArray(out), nil
}

func gsettingsSetArray(schema, key string, paths []string) error {
	if len(paths) == 0 {
		return gsettingsSet(schema, key, "[]")
	}
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = gvariantString(p)
	}
	return gsettingsSet(schema, key, "["+strings.Join(quoted, ", ")+"]")
}

func parseGSettingsStringArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "@as []" || s == "[]" {
		return nil
	}
	s = strings.TrimPrefix(s, "@as ")
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' || s[len(s)-1] != ']' {
		return nil
	}
	s = s[1 : len(s)-1]
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var paths []string
	var b strings.Builder
	var quote rune
	escape := false
	flush := func() {
		p := strings.TrimSpace(b.String())
		b.Reset()
		if p != "" {
			paths = append(paths, p)
		}
	}
	for _, r := range s {
		if escape {
			b.WriteRune(r)
			escape = false
			continue
		}
		if quote != 0 {
			if r == '\\' {
				escape = true
				continue
			}
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ',':
			flush()
		default:
			if !unicode.IsSpace(r) {
				b.WriteRune(r)
			}
		}
	}
	flush()
	return paths
}

func gsettingsCommand(schema, key string) (string, error) {
	cur, err := gsettingsGet(schema, key)
	if err != nil {
		return "", err
	}
	return unquoteGVariantString(cur), nil
}

func unquoteGVariantString(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		q := rune(s[0])
		if (q == '\'' || q == '"') && rune(s[len(s)-1]) == q {
			body := s[1 : len(s)-1]
			body = strings.ReplaceAll(body, `\'`, `'`)
			body = strings.ReplaceAll(body, `\"`, `"`)
			body = strings.ReplaceAll(body, `\\`, `\`)
			return body
		}
	}
	return s
}
