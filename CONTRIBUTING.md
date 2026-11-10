# Contributing to LinBoard

Thank you for helping make LinBoard work great on every Linux desktop!

## Development setup

```bash
git clone https://github.com/Foisalislambd/LinBoard.git
cd LinBoard
./scripts/setup.sh   # Debian/Ubuntu deps + build
make run             # uses sg input when paste group is stale
```

Auto-paste uses built-in **uinput** (`internal/clipboard/uinput_linux.go`). Check with:

```bash
linboard setup-paste
sg input -c ./linboard   # if group not active in current session
```

## Code guidelines

- Go 1.26+
- Match existing package layout under `internal/`
- Config, history, logs, and the IPC socket follow the XDG base directories (`internal/config`)
- Test on **GNOME Wayland** and at least one of: KDE, XFCE, or X11
- Paste flow: `platform.CapturePasteTarget` → `CopyClip` → `Hide` → `PasteToTarget`
- Keep hotkey logic in `internal/hotkey/` — one file per desktop environment
- Use `fyne.DoFromGoroutine` for all UI updates from background threads
- Do not log clipboard contents

## Checks

```bash
make vet
make test
make lint   # golangci-lint v2.15.0 — go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

`make test-race` race-tests the packages that do not need a display.

## Pull requests

1. Fork and create a feature branch
2. `make vet && make build`
3. Describe which desktop environment(s) you tested
4. Open a PR with a clear summary

## CI & releases

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci.yml` | Pull requests | `go mod tidy` check, vet, golangci-lint, test, race, build |
| `release.yml` | Push to `main` | Build packages → tag → GitHub Release |

Pushing to `main` auto-bumps the patch version and publishes a release **after** a successful build (tag is created last).

Skip release: include `[skip release]` in the commit message.

CLI subcommands (`version`, `help`, `setup-paste`, `doctor`) work without a display.

Each tarball contains the binary, `install.sh`, desktop file, and `QUICKSTART.txt`.

## Reporting bugs

Include:

- Distro and version (e.g. Ubuntu 24.04)
- Desktop (GNOME, KDE Plasma, XFCE, …)
- Session type (`echo $XDG_SESSION_TYPE`)
- Log output from `./linboard` and `linboard setup-paste`

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
