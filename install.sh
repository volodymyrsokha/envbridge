#!/bin/sh
# envbridge installer — https://github.com/volodymyrsokha/envbridge
# Usage: curl -fsSL https://raw.githubusercontent.com/volodymyrsokha/envbridge/main/install.sh | sh
set -eu

REPO="volodymyrsokha/envbridge"
INSTALL_DIR="${ENVBRIDGE_INSTALL_DIR:-/usr/local/bin}"

tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
[ -n "$tag" ] || { echo "could not determine the latest release" >&2; exit 1; }
version=${tag#v}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in
  darwin|linux) ;;
  *) echo "unsupported OS: $os (download manually from https://github.com/$REPO/releases)" >&2; exit 1 ;;
esac

arch=$(uname -m)
case $arch in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

asset="envbridge_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading envbridge $tag ($os/$arch)…"
curl -fsSL "$base/$asset" -o "$tmp/$asset"

curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
if command -v shasum >/dev/null 2>&1; then
  (cd "$tmp" && grep " $asset\$" checksums.txt | shasum -a 256 -c - >/dev/null)
elif command -v sha256sum >/dev/null 2>&1; then
  (cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c - >/dev/null)
else
  echo "warning: no sha256 tool found, skipping checksum verification" >&2
fi

tar -xzf "$tmp/$asset" -C "$tmp"

if [ -w "$INSTALL_DIR" ]; then
  mv "$tmp/envbridge" "$INSTALL_DIR/envbridge"
else
  echo "Writing to $INSTALL_DIR needs sudo:"
  sudo mv "$tmp/envbridge" "$INSTALL_DIR/envbridge"
fi

echo "✓ $("$INSTALL_DIR/envbridge" version) installed to $INSTALL_DIR"
