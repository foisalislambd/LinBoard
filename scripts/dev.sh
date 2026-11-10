#!/usr/bin/env bash
# LinBoard — development helper (build, clean, run, …)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY="${BINARY:-linboard}"
MAIN_PKG="./cmd/linboard"
# Version reported by `linboard version` comes from VERSION.md.
APP_VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION.md")"
LDFLAGS="${LDFLAGS:--s -w -X github.com/foisal/linboard/internal/config.AppVersion=${APP_VERSION}}"

# Prefer an explicit GOROOT, then /usr/local/go when that tree exists.
# Do not point GOROOT at a missing directory — that breaks a distro Go install.
if [[ -n "${GOROOT:-}" && -x "${GOROOT}/bin/go" ]]; then
  export PATH="$GOROOT/bin:$PATH"
elif [[ -x /usr/local/go/bin/go ]]; then
  export GOROOT=/usr/local/go
  export PATH="$GOROOT/bin:$PATH"
fi
export GOPATH="${GOPATH:-$HOME/go}"
export PATH="$GOPATH/bin:$PATH"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

cd "$ROOT"

usage() {
  cat <<'EOF'
LinBoard dev script

Usage:
  ./scripts/dev.sh <command>

Commands:
  build       Build ./linboard binary
  run         Build (if needed) and run LinBoard
  rebuild     Clean binary + full rebuild
  clean       Remove ./linboard binary
  clean-cache Clear Go build cache (slow next build)
  clean-all   Remove binary + build cache
  deps        go mod tidy && download modules
  test        Run go test ./...
  vet         Run go vet ./...
  lint        Run golangci-lint
  test-race   Run go test -race on non-GUI packages
  setup       Install system deps + build (needs sudo)
  install     Full install (~/.local/bin + autostart + Super+V)
  install-shortcut  Register Super+V only
  stop        Stop running LinBoard process
  clean-legacy  Remove old ~/.local install + legacy history.db
  help        Show this help

Examples:
  ./scripts/dev.sh build
  ./scripts/dev.sh run
  make rebuild
EOF
}

cmd_build() {
  echo "==> Building $BINARY ..."
  go build -ldflags "$LDFLAGS" -o "$BINARY" "$MAIN_PKG"
  ls -lh "$BINARY"
  echo "==> Done: $ROOT/$BINARY"
}

cmd_run() {
  if [[ ! -x "$ROOT/$BINARY" ]]; then
    cmd_build
  fi
  cmd_stop_quiet
  echo "==> Running $BINARY (Super+V to open history)"
  if [[ ! -w /dev/uinput ]] && getent group input 2>/dev/null | awk -F: '{print $4}' | tr ',' '\n' | grep -Fxq -e "$USER" && ! id -nG | tr ' ' '\n' | grep -Fxq input; then
    echo "==> input group configured but not active — starting with sg input (log out/in to skip)"
    exec sg input -c "$(printf 'exec %q' "$ROOT/$BINARY"; printf ' %q' "$@")"
  fi
  exec "$ROOT/$BINARY" "$@"
}

cmd_clean() {
  echo "==> Removing $BINARY ..."
  rm -f "$ROOT/$BINARY"
  echo "==> Clean."
}

cmd_clean_cache() {
  echo "==> Clearing Go build cache ..."
  go clean -cache -testcache
  echo "==> Cache cleared."
}

cmd_clean_all() {
  cmd_clean
  cmd_clean_cache
}

cmd_rebuild() {
  cmd_clean
  cmd_build
}

cmd_deps() {
  echo "==> Tidying modules ..."
  go mod tidy
  go mod download
  go mod verify
  echo "==> Dependencies OK."
}

cmd_test() {
  go test ./... "$@"
}

cmd_vet() {
  go vet ./...
}

cmd_lint() {
  if ! command -v golangci-lint >/dev/null 2>&1; then
    echo "golangci-lint is not installed."
    echo "Install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0"
    exit 1
  fi
  golangci-lint run ./...
}

cmd_test_race() {
  # Fyne packages pull in CGO; race-test the logic that does not need a display.
  go test -race ./internal/atomicfile ./internal/fsutil ./internal/config ./internal/store \
    ./internal/clipboard ./internal/hotkey ./internal/ipc ./internal/platform \
    ./internal/singleinstance ./internal/logging ./internal/install
}

cmd_setup() {
  "$ROOT/scripts/setup.sh"
}

cmd_install() {
  cmd_build
  "$ROOT/$BINARY" install
}

cmd_install_shortcut() {
  cmd_build
  "$ROOT/$BINARY" install-shortcut
}

cmd_stop_quiet() {
  if pgrep -x "$BINARY" >/dev/null 2>&1; then
    pkill -x "$BINARY" || true
    sleep 0.3
  fi
  if [[ -n "${XDG_RUNTIME_DIR:-}" ]]; then
    rm -f "${XDG_RUNTIME_DIR}/linboard/linboard.sock"
  fi
  rm -f \
    "${XDG_DATA_HOME:-$HOME/.local/share}/linboard/run/linboard.sock" \
    "${XDG_CONFIG_HOME:-$HOME/.config}/linboard/data/run/linboard.sock" \
    "${HOME}/.config/linboard/data/run/linboard.sock" \
    "/tmp/linboard-$(id -u)/linboard.sock"
}

cmd_stop() {
  if pgrep -x "$BINARY" >/dev/null 2>&1; then
    pkill -x "$BINARY"
    echo "==> Stopped $BINARY"
  else
    echo "==> $BINARY is not running"
  fi
  cmd_stop_quiet
}

cmd_clean_legacy() {
  cmd_stop_quiet

  local removed=0
  if [[ -x "${HOME}/.local/bin/linboard" ]]; then
    rm -f "${HOME}/.local/bin/linboard"
    echo "==> Removed ~/.local/bin/linboard"
    removed=1
  fi

  local data="${HOME}/.config/linboard/data"
  for legacy in history.db history.db.migrated history.db-wal history.db-shm; do
    if [[ -f "${data}/${legacy}" ]]; then
      rm -f "${data}/${legacy}"
      echo "==> Removed ${data}/${legacy}"
      removed=1
    fi
  done

  if [[ -f "${HOME}/.config/autostart/linboard.desktop" ]]; then
    rm -f "${HOME}/.config/autostart/linboard.desktop"
    echo "==> Removed autostart entry"
    removed=1
  fi

  if [[ -x "$ROOT/$BINARY" ]] && command -v gsettings >/dev/null 2>&1; then
    local schema="org.gnome.settings-daemon.plugins.media-keys.custom-keybinding:/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings/custom-linboard/"
    local cur
    cur="$(gsettings get "$schema" command 2>/dev/null || true)"
    if [[ "$cur" == *".local/bin/linboard"* ]]; then
      gsettings set "$schema" command "$ROOT/$BINARY toggle"
      echo "==> GNOME shortcut → $ROOT/$BINARY toggle"
      removed=1
    fi
  fi

  if [[ "$removed" -eq 0 ]]; then
    echo "==> No legacy LinBoard install found."
  else
    echo "==> Legacy cleanup done. Dev binary: $ROOT/$BINARY"
  fi
}

case "${1:-help}" in
  build)       shift; cmd_build "$@" ;;
  run)         shift; cmd_run "$@" ;;
  rebuild)     shift; cmd_rebuild "$@" ;;
  clean)       shift; cmd_clean "$@" ;;
  clean-cache) shift; cmd_clean_cache "$@" ;;
  clean-all)   shift; cmd_clean_all "$@" ;;
  deps)        shift; cmd_deps "$@" ;;
  test)        shift; cmd_test "$@" ;;
  vet)         shift; cmd_vet "$@" ;;
  lint)        shift; cmd_lint "$@" ;;
  test-race)   shift; cmd_test_race "$@" ;;
  setup)       shift; cmd_setup "$@" ;;
  install)     shift; cmd_install "$@" ;;
  install-shortcut) shift; cmd_install_shortcut "$@" ;;
  stop)        shift; cmd_stop "$@" ;;
  clean-legacy) shift; cmd_clean_legacy "$@" ;;
  help|-h|--help) usage ;;
  *)
    echo "Unknown command: $1"
    echo ""
    usage
    exit 1
    ;;
esac
