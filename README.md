# LinBoard

**Windows-style clipboard history for Linux** — press **Super+V** and pick from everything you've copied.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Latest release](https://img.shields.io/github/v/release/Foisalislambd/LinBoard?label=download)](https://github.com/Foisalislambd/LinBoard/releases)

Works on **GNOME · KDE Plasma · XFCE · Cinnamon · MATE** — **X11 and Wayland**.

---

## Install

Copy and paste this in your terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/Foisalislambd/LinBoard/main/scripts/install-release.sh | bash
```

That's it — fully automatic. The installer will:

- Detect your Linux distro and CPU (apt, dnf, pacman, zypper)
- Install dependencies (tray support, `xclip` for images, auto-paste setup)
- Configure **uinput** for click-to-paste on all desktops
- Install LinBoard, autostart, and **Super+V** shortcut
- Add `~/.local/bin` to your PATH and start LinBoard

After install, **log out and back in once** if `linboard setup-paste` asks — this activates the `input` group for auto-paste.

---

## Start using LinBoard

```bash
linboard          # start (runs in the system tray)
```

| Action | How |
|--------|-----|
| Open history | **Super+V** (Win key + V) |
| Search | Type in the search box |
| Paste item | Click or press **Enter** (auto-pastes to previous window) |
| Pin item | Press **P** |
| Delete item | Press **Delete** |

LinBoard starts automatically on login after install.

---

## Features

- Clipboard history — text, links, and images
- **Super+V** global hotkey on every major Linux desktop
- **CopyQ-style auto-paste** — built-in uinput (no ydotool/wtype needed)
- Search, pin, and keyboard navigation
- Runs quietly in the system tray
- History saved locally (JSON, your machine only)

---

## Manual install

Prefer downloading yourself? Get the package for your CPU from **[Releases](https://github.com/Foisalislambd/LinBoard/releases)**:

| File | For |
|------|-----|
| `linboard-linux-amd64-*.tar.gz` | Most PCs (Intel / AMD) |
| `linboard-linux-arm64-*.tar.gz` | Raspberry Pi 4+, ARM devices |

```bash
mkdir -p ~/linboard-install
tar -xzf linboard-linux-amd64-*.tar.gz -C ~/linboard-install
~/linboard-install/install.sh
```

---

## After install

**Command not found?** Add LinBoard to your PATH:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc
```

**Super+V not working?**

1. Make sure LinBoard is running (`linboard` or `linboard-start`)
2. Run `linboard install-shortcut`
3. Check Settings → Keyboard → look for **LinBoard**

**Paste not working?**

```bash
linboard setup-paste   # shows status + fix steps
linboard doctor        # shortcut + paste check
```

Then **log out and log back in** if prompted.

**No tray icon on GNOME?**  
Install the [AppIndicator extension](https://extensions.gnome.org/extension/615/appindicator-support/).

---

## Upgrade

Run the install command again — it always fetches the latest version and restarts LinBoard in the background:

```bash
curl -fsSL https://raw.githubusercontent.com/Foisalislambd/LinBoard/main/scripts/install-release.sh | bash
```

To restart manually after an upgrade: `linboard-start` (runs in the background; survives terminal close). LinBoard also starts on login via `~/.config/autostart/` — there is no systemd service.

---

## Uninstall

```bash
pkill linboard 2>/dev/null || true
rm -f ~/.local/bin/linboard ~/.local/bin/linboard-start
rm -f "${XDG_DATA_HOME:-$HOME/.local/share}/applications/linboard.desktop"
rm -f "${XDG_CONFIG_HOME:-$HOME/.config}/autostart/linboard.desktop"
rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/linboard"          # settings and Super+V launcher
rm -rf "${XDG_DATA_HOME:-$HOME/.local/share}/linboard"       # history and images
rm -rf "${XDG_STATE_HOME:-$HOME/.local/state}/linboard"      # logs
rm -rf ~/.config/linboard/data                               # legacy history, if still present
sudo rm -f /etc/udev/rules.d/99-linboard-uinput.rules        # optional
```

Remove the **LinBoard** entry from Settings → Keyboard if it remains.

---

## Settings

Edit `$XDG_CONFIG_HOME/linboard/config.json` (usually `~/.config/linboard/config.json`):

```json
{
  "max_history": 200,
  "start_minimized": true,
  "paste_on_select": true,
  "theme": "system"
}
```

`max_history` is clamped to 1–5000. `theme` is `system`, `light`, or `dark`. Text clips larger than 1 MiB and images larger than 20 MiB are not stored. Images must be PNG (LinBoard asks `xclip` to convert the clipboard to PNG).

| File | What's stored |
|------|-----------------|
| `$XDG_CONFIG_HOME/linboard/config.json` | Settings (mode `0600`) |
| `$XDG_DATA_HOME/linboard/clips.json` | Clipboard history (mode `0600`) |
| `$XDG_DATA_HOME/linboard/images/` | Copied images (directory mode `0700`) |
| `$XDG_STATE_HOME/linboard/linboard.log` | Logs. Clipboard contents are not written here |
| `$XDG_RUNTIME_DIR/linboard/linboard.sock` | Local control socket for Super+V (mode `0600`) |

When the XDG variables are unset, those paths are `~/.config`, `~/.local/share`, `~/.local/state`, and `/run/user/$UID`.

History written by older releases under `~/.config/linboard/data` is moved to the data directory the first time this version starts. If a history file already exists in the new location, the old copy is left in place.

---

## Build from source

LinBoard needs Go 1.27+ and cgo (Fyne). On Debian/Ubuntu:

```bash
sudo apt-get install -y gcc pkg-config \
  libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev \
  libgl1-mesa-dev libxxf86vm-dev libayatana-appindicator3-dev libdbus-1-dev
```

`./scripts/setup.sh` installs those packages and Go on Debian/Ubuntu. Other distros should install the same libraries with their package manager, then:

```bash
make test
make vet
make lint    # needs golangci-lint, see CONTRIBUTING.md
make build
./linboard version
```

`make build` stamps `linboard version` from [`VERSION.md`](VERSION.md) (a single semver, such as `1.0.13`). A bare `go build` reports `dev`.

### Cutting a release

Releases are not created on every push. Edit `VERSION.md` to a semver **strictly greater** than the latest GitHub release tag, then merge to `main`. The release workflow builds the Linux archives, creates tag `v` + that number, and publishes the GitHub release with checksums. If the number is equal to or lower than the latest release, the workflow succeeds and does not publish anything. The first release, when the repository has no semver tag yet, uses `VERSION.md` as-is.

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `linboard: command not found` | Add `~/.local/bin` to PATH (see above) |
| Super+V does nothing | Start `linboard`, then run `linboard install-shortcut` |
| Tray icon missing (GNOME) | Install [AppIndicator](https://extensions.gnome.org/extension/615/appindicator-support/) |
| Paste doesn't work | Run `linboard setup-paste`, then log out/in |
| Paste works after install but not from autostart | Use `linboard-start` (installed automatically) |
| Stops when terminal closes after upgrade | Re-run the install command, or `linboard-start` — do not run bare `linboard` in a terminal |
| Hotkey conflict | Remove the old binding in Settings → Keyboard |

### Auto-paste by desktop

| Environment | How paste works |
|-------------|-----------------|
| **GNOME / KDE / XFCE Wayland** | Built-in uinput (`input` group) |
| **X11** | Built-in uinput, or `xdotool` fallback |
| **All** | CopyQ-style: remembers window → copy → focus back → Ctrl+V |

---

## Contributing

Want to build from source or help improve LinBoard? See **[CONTRIBUTING.md](CONTRIBUTING.md)**.

## License

MIT — see [LICENSE](LICENSE).
