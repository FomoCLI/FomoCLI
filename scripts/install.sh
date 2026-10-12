#!/bin/sh
# fomo installer: downloads the latest release binary for your OS/arch.
#   curl -fsSL https://raw.githubusercontent.com/FomoCLI/FomoCLI/main/scripts/install.sh | sh
# Options (env): FOMO_VERSION=v0.1.0  FOMO_INSTALL_DIR=$HOME/.local/bin
set -eu

REPO="FomoCLI/FomoCLI"
VERSION="${FOMO_VERSION:-latest}"
DIR="${FOMO_INSTALL_DIR:-/usr/local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os (on Windows, download the .zip from GitHub releases)" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  [ -n "$VERSION" ] || { echo "could not determine latest version" >&2; exit 1; }
fi
num=${VERSION#v}
file="fomo_${num}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$VERSION"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "→ downloading fomo $VERSION ($os/$arch)"
curl -fsSL "$base/$file" -o "$tmp/$file"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

echo "→ verifying checksum"
expected=$(grep " $file\$" "$tmp/checksums.txt" | awk '{print $1}')
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$file" | awk '{print $1}')
else actual=$(shasum -a 256 "$tmp/$file" | awk '{print $1}'); fi
[ "$expected" = "$actual" ] || { echo "checksum mismatch" >&2; exit 1; }

tar -xzf "$tmp/$file" -C "$tmp"
if [ -w "$DIR" ]; then mv "$tmp/fomo" "$DIR/fomo"
else echo "→ installing to $DIR (sudo)"; sudo mv "$tmp/fomo" "$DIR/fomo"; fi
chmod +x "$DIR/fomo"
echo "✔ installed $("$DIR/fomo" --version)"
echo "  next: fomo auth login   (free key: https://fomoapi.io/dashboard)"
