#!/usr/bin/env bash
# LinBoard — install from release package (fully automatic)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${LINBOARD_BIN_DIR:-$HOME/.local/bin}"

linboard_log() { echo "==> $*"; }
DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
DESKTOP_DIR="$DATA_HOME/applications"
AUTOSTART_DIR="$CONFIG_HOME/autostart"

# shellcheck source=../lib/system-setup.sh
if [[ -f "$ROOT/lib/system-setup.sh" ]]; then
  # shellcheck disable=SC1091
  source "$ROOT/lib/system-setup.sh"
elif [[ -f "$ROOT/../lib/system-setup.sh" ]]; then
  # shellcheck disable=SC1091
  source "$ROOT/../lib/system-setup.sh"
fi

linboard_log "Installing LinBoard..."
mkdir -p "$BIN_DIR" "$DESKTOP_DIR" "$AUTOSTART_DIR"

if declare -F linboard_preflight_setup >/dev/null 2>&1; then
  linboard_preflight_setup
fi

install -m 755 "$ROOT/linboard" "$BIN_DIR/linboard"

if declare -F linboard_install_launcher >/dev/null 2>&1; then
  linboard_install_launcher "$BIN_DIR"
fi

if [[ -d "$ROOT/icons" ]]; then
  if declare -F linboard_install_theme_icons >/dev/null 2>&1; then
    linboard_install_theme_icons "$ROOT/icons"
  else
    mkdir -p "$DATA_HOME/icons/hicolor"
    cp -r "$ROOT/icons/hicolor/." "$DATA_HOME/icons/hicolor/" 2>/dev/null || true
  fi
fi

LAUNCHER="$BIN_DIR/linboard-start"
[[ -x "$LAUNCHER" ]] || LAUNCHER="$BIN_DIR/linboard"
# Quote Exec when the path contains characters the desktop entry spec splits on.
desktop_exec="$LAUNCHER"
case "$LAUNCHER" in
  *[[:space:]]*|*[\"\\\$\`]*)
    escaped=${LAUNCHER//\\/\\\\}
    escaped=${escaped//\"/\\\"}
    escaped=${escaped//\$/\\\$}
    escaped=${escaped//\`/\\\`}
    desktop_exec="\"$escaped\""
    ;;
esac
while IFS= read -r line || [[ -n "$line" ]]; do
  printf '%s\n' "${line/@EXEC@/$desktop_exec}"
done < "$ROOT/linboard.desktop" > "$DESKTOP_DIR/linboard.desktop"
chmod 644 "$DESKTOP_DIR/linboard.desktop"

START_EXEC="$BIN_DIR/linboard-start"
[[ -x "$START_EXEC" ]] || START_EXEC="$BIN_DIR/linboard"
case "$START_EXEC" in
  *[[:space:]]*|*[\"\\\$\`]*)
    escaped=${START_EXEC//\\/\\\\}
    escaped=${escaped//\"/\\\"}
    escaped=${escaped//\$/\\\$}
    escaped=${escaped//\`/\\\`}
    start_exec="\"$escaped\""
    ;;
  *) start_exec="$START_EXEC" ;;
esac
cat > "$AUTOSTART_DIR/linboard.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=LinBoard
GenericName=Clipboard Manager
Comment=Windows-style clipboard history for Linux
Exec=$start_exec
Icon=linboard
Terminal=false
Categories=Utility;
StartupNotify=false
X-GNOME-Autostart-enabled=true
EOF

if declare -F linboard_post_install_setup >/dev/null 2>&1; then
  linboard_post_install_setup "$BIN_DIR"
else
  "$BIN_DIR/linboard" install-shortcut || true
  echo "Installed: $BIN_DIR/linboard — press Super+V"
fi
