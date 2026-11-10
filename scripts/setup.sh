#!/usr/bin/env bash
# LinBoard — system setup for Ubuntu/Debian
set -euo pipefail

GO_VERSION="1.27.3"
case "$(uname -m)" in
  x86_64)  GO_ARCH="linux-amd64" ;;
  aarch64) GO_ARCH="linux-arm64" ;;
  *)
    echo "Unsupported CPU: $(uname -m) (need x86_64 or aarch64)"
    exit 1
    ;;
esac
GO_TAR="go${GO_VERSION}.${GO_ARCH}.tar.gz"
GO_URL="https://go.dev/dl/${GO_TAR}"
GO_SHA_URL="https://go.dev/dl/${GO_TAR}.sha256"

echo "==> Installing build dependencies..."
sudo apt-get update -qq
sudo apt-get install -y \
  gcc libc6-dev pkg-config \
  libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev \
  libgl1-mesa-dev libxxf86vm-dev \
  libayatana-appindicator3-dev libdbus-1-dev \
  xdotool

echo "==> Checking Go installation..."
CURRENT=$(go version 2>/dev/null | awk '{print $3}' | sed 's/go//' || echo "none")
if [ "$CURRENT" = "$GO_VERSION" ]; then
  echo "    Go $GO_VERSION already installed at $(which go)"
else
  echo "    Installing Go $GO_VERSION to /usr/local/go ..."
  tmp=$(mktemp -d)
  curl -fsSL "$GO_URL" -o "$tmp/$GO_TAR"
  GO_SHA="$(curl -fsSL "$GO_SHA_URL" | awk '{print $1}')"
  echo "$GO_SHA  $tmp/$GO_TAR" | sha256sum -c -
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$tmp/$GO_TAR"
  rm -rf "$tmp"
fi

BASHRC="$HOME/.bashrc"
MARKER="# LinBoard Go setup"
if ! grep -q "$MARKER" "$BASHRC" 2>/dev/null; then
  echo "==> Configuring shell environment in ~/.bashrc ..."
  cat >> "$BASHRC" <<'EOF'

# LinBoard Go setup
export GOROOT=/usr/local/go
export GOPATH="$HOME/go"
export GOTOOLCHAIN=local
export PATH="$GOROOT/bin:$GOPATH/bin:$PATH"
EOF
fi

export GOROOT=/usr/local/go
export GOPATH="${GOPATH:-$HOME/go}"
export GOTOOLCHAIN=local
export PATH="$GOROOT/bin:$GOPATH/bin:$PATH"

echo "==> Go environment:"
go version
go env GOROOT GOPATH

echo "==> Building LinBoard..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."
go mod tidy
APP_VERSION="$(tr -d '[:space:]' < VERSION.md)"
go build -ldflags "-s -w -X github.com/foisal/linboard/internal/config.AppVersion=${APP_VERSION}" -o linboard ./cmd/linboard

echo ""
echo "Done! Run: ./linboard"
echo "Hotkey: Super+V (Win key + V)"
