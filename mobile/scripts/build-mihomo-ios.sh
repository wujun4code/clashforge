#!/usr/bin/env bash
# Builds Mihomobridge.xcframework (mihomo core + iOS bridge) via gomobile.
# Requires macOS with Xcode — run on CI (ios-ci.yml) or a rented Mac; the
# output lands in mobile/ios/Frameworks/ which the PacketTunnel target links.
#
# Usage: bash mobile/scripts/build-mihomo-ios.sh
set -euo pipefail

MOBILE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BRIDGE_DIR="$MOBILE_DIR/ios/MihomoBridge"
OUT_DIR="$MOBILE_DIR/ios/Frameworks"
OUT="$OUT_DIR/Mihomobridge.xcframework"

GOMOBILE_VERSION="$(go list -m -f '{{.Version}}' -C "$BRIDGE_DIR" golang.org/x/mobile)"
echo "==> Installing gomobile/gobind ${GOMOBILE_VERSION} (pinned by MihomoBridge/go.mod)"
go install "golang.org/x/mobile/cmd/gomobile@${GOMOBILE_VERSION}"
go install "golang.org/x/mobile/cmd/gobind@${GOMOBILE_VERSION}"
export PATH="$PATH:$(go env GOPATH)/bin"

mkdir -p "$OUT_DIR"
rm -rf "$OUT"

echo "==> gomobile bind (ios/arm64, min iOS 15.0)"
cd "$BRIDGE_DIR"
# with_gvisor matches the Android core build; the iOS config defaults to
# stack: system but keeps gvisor available as a one-line config fallback.
gomobile bind \
  -target ios \
  -iosversion 15.0 \
  -tags with_gvisor \
  -trimpath \
  -ldflags="-s -w" \
  -o "$OUT" \
  .

echo "==> Built: $OUT"
du -sh "$OUT"
