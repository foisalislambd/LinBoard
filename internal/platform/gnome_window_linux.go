//go:build linux

package platform

import (
	"os/exec"
	"strings"
)

func gnomeCaptureFocus() string {
	if !IsGNOME() || !UsePortalHotkey() {
		return ""
	}
	ok, val := gnomeEval(`(global.display.focus_window ? String(global.display.focus_window.get_stable_sequence()) : '')`)
	if !ok || val == "" {
		return ""
	}
	return val
}

func gnomeRestoreFocus(seq string) {
	if seq == "" {
		return
	}
	for _, r := range seq {
		if r < '0' || r > '9' {
			return
		}
	}
	js := `(function(seq) {
  const n = parseInt(seq, 10);
  for (const a of global.get_window_actors()) {
    const w = a.metaWindow;
    if (w.get_stable_sequence() === n) {
      w.activate(global.get_current_time());
      return 'ok';
    }
  }
  return '';
})(` + seq + `)`
	_, _ = gnomeEval(js)
}

func gnomeEval(js string) (bool, string) {
	if _, err := exec.LookPath("gdbus"); err != nil {
		return false, ""
	}
	out, err := exec.Command(
		"gdbus", "call", "--session",
		"--dest", "org.gnome.Shell",
		"--object-path", "/org/gnome/Shell",
		"--method", "org.gnome.Shell.Eval",
		js,
	).Output()
	if err != nil {
		return false, ""
	}
	return parseGnomeEval(string(out))
}

func parseGnomeEval(out string) (bool, string) {
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "(true,") {
		return false, ""
	}
	i := strings.Index(out, "'")
	j := strings.LastIndex(out, "'")
	if i < 0 || j <= i {
		return false, ""
	}
	val := out[i+1 : j]
	val = strings.ReplaceAll(val, `\'`, `'`)
	return true, val
}
