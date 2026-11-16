#!/usr/bin/env bash
# LinBoard — automatic system detection & dependency setup
# Sourced by install-release.sh and package/install.sh

linboard_log() { echo "==> $*"; }
linboard_warn() { echo "==> [note] $*" >&2; }

linboard_detect_os() {
  LINBOARD_OS_ID=""
  LINBOARD_OS_LIKE=""
  LINBOARD_PM=""   # apt, dnf, pacman, zypper

  if [[ -f /etc/os-release ]]; then
    # Parse without sourcing — /etc/os-release sets VERSION, ID, etc. and would
    # clobber the installer's release version variable.
    LINBOARD_OS_ID="$(grep -E '^ID=' /etc/os-release | head -1 | cut -d= -f2- | tr -d '"')"
    LINBOARD_OS_LIKE="$(grep -E '^ID_LIKE=' /etc/os-release | head -1 | cut -d= -f2- | tr -d '"')"
    LINBOARD_OS_ID="${LINBOARD_OS_ID:-unknown}"
  fi

  case "$LINBOARD_OS_ID" in
    ubuntu|debian|linuxmint|pop|zorin|elementary|neon)
      LINBOARD_PM="apt" ;;
    fedora|nobara)
      LINBOARD_PM="dnf" ;;
    rhel|centos|rocky|almalinux|ol)
      LINBOARD_PM="dnf" ;;
    arch|manjaro|endeavouros|garuda)
      LINBOARD_PM="pacman" ;;
    opensuse-leap|opensuse-tumbleweed|opensuse|sles|sled)
      LINBOARD_PM="zypper" ;;
    *)
      if [[ "$LINBOARD_OS_LIKE" == *debian* ]] || [[ "$LINBOARD_OS_LIKE" == *ubuntu* ]]; then
        LINBOARD_PM="apt"
      elif [[ "$LINBOARD_OS_LIKE" == *fedora* ]] || [[ "$LINBOARD_OS_LIKE" == *rhel* ]]; then
        LINBOARD_PM="dnf"
      elif [[ "$LINBOARD_OS_LIKE" == *arch* ]]; then
        LINBOARD_PM="pacman"
      fi
      ;;
  esac
}

linboard_detect_session() {
  LINBOARD_SESSION="${XDG_SESSION_TYPE:-unknown}"
  LINBOARD_DESKTOP="${XDG_CURRENT_DESKTOP:-}"
  LINBOARD_DESKTOP_LC="$(echo "$LINBOARD_DESKTOP" | tr '[:upper:]' '[:lower:]')"
}

linboard_have() { command -v "$1" >/dev/null 2>&1; }

linboard_bytes_to_mb() {
  awk "BEGIN {printf \"%.2f\", ${1:-0}/1048576}"
}

# Download with size hint and live progress bar (when stdout is a terminal).
linboard_download() {
  local url="$1"
  local dest="$2"
  local label="${3:-$(basename "$dest")}"

  local total_bytes=""
  total_bytes="$(curl -fsSLI -A "LinBoard-Installer" "$url" 2>/dev/null | grep -i '^content-length:' | tail -1 | awk '{print $2}' | tr -d '\r')" || true

  if [[ -n "$total_bytes" && "$total_bytes" =~ ^[0-9]+$ ]]; then
    linboard_log "Downloading ${label} ($(linboard_bytes_to_mb "$total_bytes") MB)..."
  else
    linboard_log "Downloading ${label}..."
  fi

  if [[ -t 1 ]]; then
    # Live progress bar (speed + % when Content-Length known)
    curl -fL -A "LinBoard-Installer" --progress-bar "$url" -o "$dest"
    echo ""
  else
    curl -fSL -A "LinBoard-Installer" "$url" -o "$dest"
  fi

  if [[ -f "$dest" ]]; then
    local size
    size="$(stat -c%s "$dest" 2>/dev/null || stat -f%z "$dest")"
    linboard_log "Download complete: $(linboard_bytes_to_mb "$size") MB"
  fi
}

linboard_sudo() {
  if linboard_have sudo; then
    sudo "$@"
  elif [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  else
    return 1
  fi
}

linboard_install_packages() {
  local pkgs=("$@")
  [[ ${#pkgs[@]} -eq 0 ]] && return 0

  linboard_detect_os
  linboard_log "Installing system packages: ${pkgs[*]} (${LINBOARD_PM:-unknown distro})"

  case "$LINBOARD_PM" in
    apt)
      linboard_sudo apt-get update -qq
      linboard_sudo apt-get install -y "${pkgs[@]}"
      ;;
    dnf)
      linboard_sudo dnf install -y "${pkgs[@]}"
      ;;
    pacman)
      linboard_sudo pacman -Sy --noconfirm "${pkgs[@]}"
      ;;
    zypper)
      linboard_sudo zypper --non-interactive install "${pkgs[@]}"
      ;;
    *)
      linboard_warn "Could not auto-install packages on this distro."
      linboard_warn "Please install manually: ${pkgs[*]}"
      return 1
      ;;
  esac
}

linboard_install_clipboard_tools() {
  linboard_detect_session
  local need=()

  # Text clipboard uses LinBoard's Fyne/GLFW core on Wayland (no wl-clipboard).
  # xclip is optional: image history via XWayland on GNOME, full clipboard on X11.
  if ! linboard_have xclip && ! linboard_have xsel; then need+=("xclip"); fi

  [[ ${#need[@]} -eq 0 ]] && return 0

  linboard_log "Setting up clipboard tools (${LINBOARD_SESSION} session)..."
  linboard_install_packages "${need[@]}" || true
}

linboard_ensure_uinput_module() {
  [[ -e /dev/uinput ]] && return 0
  if linboard_have modprobe; then
    linboard_log "Loading uinput kernel module..."
    linboard_sudo modprobe uinput 2>/dev/null || true
  fi
}

linboard_ensure_uinput_udev_rule() {
  local rule_file="/etc/udev/rules.d/99-linboard-uinput.rules"
  local rule='KERNEL=="uinput", GROUP="input", MODE="0660", OPTIONS+="static_node=uinput"'

  if [[ -f "$rule_file" ]] && grep -qF 'GROUP="input"' "$rule_file" 2>/dev/null; then
    return 0
  fi

  linboard_log "Installing uinput udev rule for auto-paste..."
  if ! linboard_sudo tee "$rule_file" >/dev/null <<EOF
# LinBoard — allow input group to use uinput for auto-paste
$rule
EOF
  then
    linboard_warn "Could not install $rule_file — run: linboard setup-paste"
    return 1
  fi

  if command -v udevadm >/dev/null 2>&1; then
    linboard_sudo udevadm control --reload-rules 2>/dev/null || true
    linboard_sudo udevadm trigger --subsystem-match=misc 2>/dev/null || true
  fi
}

linboard_ensure_input_group() {
  [[ -e /dev/uinput ]] || return 0
  [[ -w /dev/uinput ]] && return 0

  linboard_ensure_uinput_udev_rule || true

  if id -nG "$USER" 2>/dev/null | tr ' ' '\n' | grep -Fxq input; then
    linboard_warn "input group set but /dev/uinput not writable — log out and back in"
    return 0
  fi

  linboard_log "Adding $USER to input group for auto-paste..."
  linboard_sudo usermod -aG input "$USER" || true
  linboard_warn "Log out and log back in for auto-paste to work"
}

# sg (from util-linux-extra on Ubuntu 24.04+) lets this login use the input
# group without a logout. The app still starts when the package is missing.
linboard_ensure_sg() {
  linboard_have sg && return 0
  linboard_detect_os
  case "${LINBOARD_PM:-}" in
    apt)
      linboard_install_packages util-linux-extra || true
      ;;
  esac
}

linboard_install_paste_tools() {
  linboard_detect_session
  linboard_ensure_uinput_module
  linboard_ensure_uinput_udev_rule || true
  linboard_ensure_input_group
  linboard_ensure_sg || true

  # X11 sessions: xdotool helps focus + paste for legacy apps.
  if [[ "$LINBOARD_SESSION" == "x11" ]] && ! linboard_have xdotool; then
    linboard_log "Installing xdotool (X11 auto-paste fallback)..."
    linboard_install_packages xdotool || true
  fi
}

linboard_install_launcher() {
  local bin_dir="${1:-$HOME/.local/bin}"
  local launcher="$bin_dir/linboard-start"
  local exe="$bin_dir/linboard"
  [[ -x "$exe" ]] || return 0

  cat > "$launcher" <<'EOF'
#!/usr/bin/env bash
# LinBoard launcher — joins the input group when this login does not have it yet.
# A missing or failing sg must still start the app (autostart and the installer
# both call this script). Do not use `set -e`: a failed sg has to fall through.
set -uo pipefail

# Detach when run from a terminal so the app survives after the shell closes.
if [[ -t 1 ]] && [[ "${LINBOARD_FOREGROUND:-}" != "1" ]]; then
  export LINBOARD_FOREGROUND=1
  if command -v setsid >/dev/null 2>&1; then
    setsid "$0" "$@" </dev/null >/dev/null 2>&1 &
  else
    nohup "$0" "$@" </dev/null >/dev/null 2>&1 &
  fi
  disown 2>/dev/null || true
  exit 0
fi

LB="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/linboard"
if [[ ! -x "$LB" && -x "$HOME/.local/bin/linboard" ]]; then
  LB="$HOME/.local/bin/linboard"
fi
if [[ ! -x "$LB" ]]; then
  echo "linboard: binary not found" >&2
  exit 1
fi

if [[ -w /dev/uinput ]]; then
  exec "$LB" "$@"
fi

me="${USER:-$(id -un)}"
if command -v sg >/dev/null 2>&1 \
  && getent group input 2>/dev/null | awk -F: '{print $4}' | tr ',' '\n' | grep -Fxq -e "$me" \
  && ! id -nG | tr ' ' '\n' | grep -Fxq input; then
  # Do not exec sg. If it is missing or refuses the group, start the binary anyway.
  sg input -c "$(printf 'exec %q ' "$LB" "$@")" && exit 0
fi

exec "$LB" "$@"
EOF
  chmod +x "$launcher"
}

linboard_ensure_gnome_media_keys() {
  linboard_detect_session
  [[ "$LINBOARD_DESKTOP_LC" != *gnome* ]] && return 0

  if busctl --user status org.gnome.SettingsDaemon.MediaKeys >/dev/null 2>&1; then
    return 0
  fi

  if [[ -x /usr/libexec/gsd-media-keys ]]; then
    linboard_log "Starting GNOME media-keys (required for Super+V)..."
    /usr/libexec/gsd-media-keys >/dev/null 2>&1 &
    disown 2>/dev/null || true
    sleep 0.5
  fi

  if ! busctl --user status org.gnome.SettingsDaemon.MediaKeys >/dev/null 2>&1; then
    linboard_warn "gsd-media-keys not running — install gnome-settings-daemon if Super+V fails"
  fi
}

linboard_install_gnome_tray() {
  linboard_detect_session
  [[ "$LINBOARD_DESKTOP_LC" != *gnome* ]] && return 0

  # Already have tray support?
  if linboard_have gnome-extensions; then
    if gnome-extensions list --enabled 2>/dev/null | grep -qi appindicator; then
      return 0
    fi
  fi

  linboard_log "GNOME detected — installing tray icon support..."
  linboard_detect_os

  local pkg=""
  case "$LINBOARD_PM" in
    apt)  pkg="gnome-shell-extension-appindicator" ;;
    dnf)  pkg="gnome-shell-extension-appindicator" ;;
    pacman) pkg="gnome-shell-extension-appindicator" ;;
  esac

  if [[ -n "$pkg" ]]; then
    linboard_install_packages "$pkg" || true
  fi

  # Try to enable the extension when the CLI is available
  if linboard_have gnome-extensions; then
    local ext_uuid=""
    for uuid in \
      "appindicatorsupport@rgcjonas.gmail.com" \
      "AppIndicatorExtension@martin.zimmermann"; do
      if gnome-extensions list 2>/dev/null | grep -qF "$uuid"; then
        ext_uuid="$uuid"
        break
      fi
    done
    if [[ -n "$ext_uuid" ]]; then
      gnome-extensions enable "$ext_uuid" 2>/dev/null || true
      linboard_log "Enabled GNOME AppIndicator extension"
    fi
  fi
}

linboard_install_theme_icons() {
  local icons_root="$1"
  [[ -d "$icons_root/hicolor" ]] || return 0

  local dest="${XDG_DATA_HOME:-$HOME/.local/share}/icons"
  mkdir -p "$dest/hicolor"

  linboard_log "Installing application icon..."
  cp -r "$icons_root/hicolor/." "$dest/hicolor/"

  if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t "$dest" >/dev/null 2>&1 || true
  fi
  if command -v update-icon-caches >/dev/null 2>&1; then
    update-icon-caches "$dest" >/dev/null 2>&1 || true
  fi
}

linboard_ensure_path() {
  local bin_dir="$1"
  [[ -z "$bin_dir" ]] && return 0
  [[ ":$PATH:" == *":$bin_dir:"* ]] && return 0

  linboard_log "Adding $bin_dir to PATH..."
  local line="export PATH=\"$bin_dir:\$PATH\""
  local updated=0

  for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
    [[ -f "$rc" ]] || continue
    if ! grep -qF "$bin_dir" "$rc" 2>/dev/null; then
      printf '\n# LinBoard\n%s\n' "$line" >> "$rc"
      updated=1
    fi
  done

  if [[ "$updated" -eq 0 ]]; then
    touch "$HOME/.profile"
    if ! grep -qF "$bin_dir" "$HOME/.profile" 2>/dev/null; then
      printf '\n# LinBoard\n%s\n' "$line" >> "$HOME/.profile"
    fi
  fi

  export PATH="$bin_dir:$PATH"
}

linboard_app_running() {
  pgrep -x linboard >/dev/null 2>&1
}

linboard_wait_until_running() {
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    linboard_app_running && return 0
    sleep 0.3
  done
  return 1
}

linboard_spawn_detached() {
  local cmd="$1"
  if command -v setsid >/dev/null 2>&1; then
    setsid "$cmd" </dev/null >/dev/null 2>&1 &
  else
    nohup "$cmd" </dev/null >/dev/null 2>&1 &
  fi
  disown 2>/dev/null || true
}

linboard_start_app() {
  local bin_dir="$1"
  local exe="$bin_dir/linboard"
  local launcher="$bin_dir/linboard-start"

  [[ -x "$exe" ]] || return 0
  linboard_install_launcher "$bin_dir"

  if [[ -z "${DISPLAY:-}" && -z "${WAYLAND_DISPLAY:-}" ]]; then
    linboard_warn "No display detected — LinBoard will start on next login"
    return 0
  fi

  if linboard_app_running; then
    linboard_log "Restarting LinBoard..."
    pkill -x linboard 2>/dev/null || true
    sleep 0.4
  fi

  linboard_log "Starting LinBoard in the background..."

  # Login autostart is a systemd user unit generated from the desktop file.
  # It is not started for a file added during an already-open session, so
  # start that unit now. The next login starts it the same way.
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload >/dev/null 2>&1 || true
    systemctl --user reset-failed app-linboard@autostart.service >/dev/null 2>&1 || true
    systemctl --user start app-linboard@autostart.service >/dev/null 2>&1 || true
    if linboard_wait_until_running; then
      linboard_log "LinBoard is running (system tray) and will start on login"
      return 0
    fi
  fi

  local run="$exe"
  [[ -x "$launcher" ]] && run="$launcher"
  linboard_spawn_detached "$run"
  if linboard_wait_until_running; then
    linboard_log "LinBoard is running (system tray) and will start on login"
    return 0
  fi

  if [[ "$run" != "$exe" ]]; then
    linboard_warn "Launcher exited — starting the LinBoard binary directly"
    linboard_spawn_detached "$exe"
    if linboard_wait_until_running; then
      linboard_log "LinBoard is running (system tray) and will start on login"
      return 0
    fi
  fi

  linboard_warn "Could not start LinBoard. Run: $exe"
}

# Run before downloading / installing LinBoard binary (once per install run)
linboard_preflight_setup() {
  if [[ "${LINBOARD_PREFLIGHT_DONE:-}" == "1" ]]; then
    return 0
  fi
  export LINBOARD_PREFLIGHT_DONE=1

  linboard_detect_os
  linboard_detect_session

  linboard_log "Detected: ${LINBOARD_OS_ID:-Linux} | ${LINBOARD_DESKTOP:-desktop} | ${LINBOARD_SESSION:-session}"

  if ! linboard_have curl; then
    linboard_install_packages curl || true
  fi

  linboard_install_clipboard_tools
  linboard_install_paste_tools
  linboard_install_gnome_tray
  linboard_ensure_gnome_media_keys
}

# Run after binary is installed to ~/.local/bin
linboard_post_install_setup() {
  local bin_dir="${1:-$HOME/.local/bin}"

  linboard_ensure_path "$bin_dir"

  if [[ -n "${DISPLAY:-}" ]] || [[ -n "${WAYLAND_DISPLAY:-}" ]]; then
    if "$bin_dir/linboard" install-shortcut; then
      linboard_log "Super+V shortcut registered"
    else
      linboard_warn "Shortcut setup failed — run: linboard doctor"
    fi
  else
    linboard_warn "No display — run 'linboard install-shortcut' after login"
  fi

  linboard_install_launcher "$bin_dir"
  linboard_start_app "$bin_dir"

  if [[ -e /dev/uinput ]] && ! [[ -w /dev/uinput ]]; then
    linboard_warn "Auto-paste: log out and back in (or run: linboard setup-paste)"
  fi

  echo ""
  echo "  ┌─────────────────────────────────────────────┐"
  echo "  │  LinBoard installed successfully!           │"
  echo "  │                                             │"
  echo "  │  Press Super+V (Win+V) to open history      │"
  echo "  │  Click an item to paste (CopyQ-style)       │"
  echo "  │  Check paste: linboard setup-paste          │"
  echo "  └─────────────────────────────────────────────┘"
  echo ""
}
