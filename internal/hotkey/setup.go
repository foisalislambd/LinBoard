package hotkey

import (
	"fmt"
	"os"
	"strings"

	"github.com/foisal/linboard/internal/clipboard"
	"github.com/foisal/linboard/internal/config"
	"github.com/foisal/linboard/internal/platform"
)

// SetupAt registers Super+V for exe (idempotent). Call from install and app start.
func SetupAt(exe string) error {
	if platform.IsGNOME() {
		return setupGNOME(exe)
	}
	if platform.IsMATE() {
		return setupMATEHotkey(exe)
	}
	return registerSystemShortcut(exe)
}

// VerifyReport is the result of post-install shortcut checks.
type VerifyReport struct {
	OK     []string
	Warn   []string
	Fail   []string
	Binary string
}

func (r VerifyReport) Healthy() bool {
	return len(r.Fail) == 0
}

// Verify checks shortcut prerequisites after SetupAt.
func Verify(exe string) (r VerifyReport) {
	r = VerifyReport{Binary: exe}
	// Named return so defer can append paste checks to the value actually returned.
	defer appendPasteVerify(&r)

	switch platform.CurrentDesktop() {
	case platform.DesktopGNOME:
		r = verifyGNOMEShortcut(r, exe)
	case platform.DesktopMATE:
		r = verifyMATEShortcut(r, exe)
	case platform.DesktopXFCE:
		r = verifyXFCEShortcut(r, exe)
	case platform.DesktopKDE:
		r = verifyKDEShortcut(r, exe)
	case platform.DesktopCinnamon:
		r = verifyCinnamonShortcut(r, exe)
	default:
		r.OK = append(r.OK, "desktop: "+platform.DesktopName())
		r.Warn = append(r.Warn, "automatic shortcut verify not available for this desktop — test Super+V manually")
	}
	return
}

func verifyGNOMEShortcut(r VerifyReport, exe string) VerifyReport {
	if !hasBin("gsettings") {
		r.Fail = append(r.Fail, "gsettings not found")
		return r
	}

	wrapper, err := ToggleWrapperPath()
	if err != nil {
		r.Fail = append(r.Fail, err.Error())
		return r
	}

	if st, err := os.Stat(wrapper); err != nil || st.Mode()&0o111 == 0 {
		r.Fail = append(r.Fail, "toggle launcher missing: "+wrapper)
	} else {
		r.OK = append(r.OK, "toggle launcher: "+wrapper)
	}

	schema := gnomeBindingSchema()
	cmd, err := gsettingsCommand(schema, "command")
	if err != nil {
		r.Fail = append(r.Fail, "gsettings shortcut not registered")
	} else if cmd != wrapper {
		r.Warn = append(r.Warn, fmt.Sprintf("gsettings command is %q (want %q)", cmd, wrapper))
	} else {
		r.OK = append(r.OK, "gsettings command OK")
	}

	bind, err := gsettingsGet(schema, "binding")
	if err != nil {
		r.Fail = append(r.Fail, "shortcut binding not set")
	} else if strings.Trim(bind, "'") != "<Super>v" {
		r.Warn = append(r.Warn, "binding is "+bind+" (want <Super>v)")
	} else {
		r.OK = append(r.OK, "binding: Super+V")
	}

	paths, err := gsettingsListPaths()
	if err != nil || !containsPath(paths, gnomeBindingPath()) {
		r.Fail = append(r.Fail, "custom-linboard not in media-keys list")
	} else {
		r.OK = append(r.OK, "registered in GNOME media-keys")
	}

	if mediaKeysRunning() {
		r.OK = append(r.OK, "gsd-media-keys running")
	} else {
		r.Fail = append(r.Fail, "gsd-media-keys not running (Super+V will not work)")
	}

	tray, err := gsettingsGetArray(gnomeShellKeybindings, gnomeMessageTrayKey)
	if err == nil {
		for _, b := range tray {
			if strings.EqualFold(b, "<Super>v") {
				r.Warn = append(r.Warn, "GNOME message tray still uses Super+V — run install-shortcut again")
				break
			}
		}
	}

	return r
}

func verifyKDEShortcut(r VerifyReport, exe string) VerifyReport {
	r.OK = append(r.OK, "desktop: KDE Plasma")
	path := kdeHotkeysPath()
	content := readFile(path)
	if content == "" {
		r.Fail = append(r.Fail, "khotkeysrc missing — Super+V may not be registered")
		return r
	}
	if !strings.Contains(content, "linboard-toggle") {
		r.Fail = append(r.Fail, "LinBoard entry not found in khotkeysrc")
		return r
	}
	want := exe + " toggle"
	if !strings.Contains(content, want) {
		r.Warn = append(r.Warn, fmt.Sprintf("khotkeys command may be stale (want %q)", want))
	} else {
		r.OK = append(r.OK, "khotkeys command OK")
	}
	if !strings.Contains(content, "Key=Meta+V") {
		r.Warn = append(r.Warn, "Meta+V binding not found in khotkeysrc")
	} else {
		r.OK = append(r.OK, "binding: Meta+V")
	}
	return r
}

func verifyCinnamonShortcut(r VerifyReport, exe string) VerifyReport {
	r.OK = append(r.OK, "desktop: Cinnamon")
	if !hasBin("gsettings") {
		r.Fail = append(r.Fail, "gsettings not found")
		return r
	}
	listSchema := "org.cinnamon.desktop.keybindings"
	path := "/org/cinnamon/desktop/keybindings/custom-keybindings/custom-linboard/"
	fullSchema := listSchema + ".custom-keybinding:" + path

	paths, err := gsettingsGetArray(listSchema, "custom-keybindings")
	if err != nil || len(paths) == 0 {
		paths, err = gsettingsGetArray(listSchema, "custom-list")
	}
	if err != nil || !containsPath(paths, path) {
		r.Fail = append(r.Fail, "custom-linboard not in Cinnamon keybindings list")
		return r
	}
	r.OK = append(r.OK, "registered in Cinnamon keybindings")

	cmd, err := gsettingsCommand(fullSchema, "command")
	want := exe + " toggle"
	if err != nil {
		r.Fail = append(r.Fail, "Cinnamon shortcut command not set")
	} else if cmd != want {
		r.Warn = append(r.Warn, fmt.Sprintf("Cinnamon command is %q (want %q)", cmd, want))
	} else {
		r.OK = append(r.OK, "Cinnamon command OK")
	}
	bind, err := gsettingsGet(fullSchema, "binding")
	if err != nil {
		r.Fail = append(r.Fail, "Cinnamon shortcut binding not set")
	} else if strings.Trim(bind, "'") != "<Super>v" {
		r.Warn = append(r.Warn, "binding is "+bind+" (want <Super>v)")
	} else {
		r.OK = append(r.OK, "binding: Super+V")
	}
	return r
}

func appendPasteVerify(r *VerifyReport) {
	if clipboard.PasteReady() {
		r.OK = append(r.OK, "auto-paste ready ("+clipboard.PasteToolName()+")")
		return
	}
	if clipboard.SessionNeedsRelogin() {
		r.Warn = append(r.Warn, "auto-paste: log out/in to activate input group (or: linboard-start)")
		return
	}
	r.Warn = append(r.Warn, "auto-paste not ready — run: linboard setup-paste")
	if !platform.UsePortalHotkey() && !hasBin("xdotool") {
		r.Warn = append(r.Warn, "X11 fallback: install xdotool if uinput setup is unavailable")
	}
}

// PrintVerify writes a human-readable report to stdout.
func PrintVerify(r VerifyReport) {
	fmt.Println()
	fmt.Println("Shortcut check (" + config.HotkeyLabel + "):")
	for _, s := range r.OK {
		fmt.Println("  ✓", s)
	}
	for _, s := range r.Warn {
		fmt.Println("  !", s)
	}
	for _, s := range r.Fail {
		fmt.Println("  ✗", s)
	}
	if r.Healthy() {
		fmt.Println()
		fmt.Println("Super+V is ready. Press Win+V to open clipboard history.")
	} else {
		fmt.Println()
		fmt.Println("Fix issues above, then run: linboard install-shortcut")
	}
	fmt.Println()
}
