#!/usr/bin/env bash
# Prepare mihomo binaries for Android ABIs and place as jniLibs.
#
# ── Background ────────────────────────────────────────────────────────────────
#
# Two problems exist with pre-built mihomo android binaries that both need
# to be solved simultaneously:
#
# Problem 1 — GOOS=linux binaries (e.g. linux-arm64):
#   After opening the TUN fd they try to reconfigure the interface via
#   ioctl/netlink (set MTU, IP address, routes).  This requires CAP_NET_ADMIN,
#   which Android VPN apps don't have.  Android VpnService already fully
#   configures the interface; we must use the fd as-is.
#   Error: "configure tun interface: permission denied"
#
# Problem 2 — GOOS=android binaries (v1.18+):
#   Call buildAndroidRules() unconditionally during TUN init, which reads
#   /data/system/packages.xml.  SELinux denies this on non-rooted devices.
#   Error: "build android rules: read packages list: ... permission denied"
#
# ── Correct solution ──────────────────────────────────────────────────────────
#
# Build mihomo with:
#   GOOS=android  — correct VPN fd path (VpnService already configured the
#                   interface, no CAP_NET_ADMIN calls needed)
#   -tags cmfa    — buildAndroidRules() becomes a no-op stub; same approach
#                   used by ClashMetaForAndroid (CMFA)
#
# This script builds from source when Go is available (recommended for arm64-v8a
# real-device testing).  For x86_64 emulator-only testing it downloads the
# android-amd64 pre-built binary since the CI emulator has a chmod workaround.
#
# Usage (from repo root):
#   bash mobile/scripts/download-mihomo.sh          # download-only (emulator)
#   bash mobile/scripts/download-mihomo.sh --build  # build from source (real device)
#
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

MIHOMO_VER="v1.18.10"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JNILIBS_ROOT="$SCRIPT_DIR/../android/app/src/main/jniLibs"
BUILD_MODE="${1:-}"  # pass --build to compile from source

# ── Build from source (GOOS=android -tags cmfa) ──────────────────────────────
if [ "$BUILD_MODE" = "--build" ]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "ERROR: --build requires Go. Install from https://go.dev/dl/"
    exit 1
  fi

  SRC="/tmp/mihomo-src-${MIHOMO_VER}"
  if [ ! -d "$SRC" ]; then
    echo "Cloning mihomo ${MIHOMO_VER}…"
    git clone --depth 1 --branch "${MIHOMO_VER}" \
      https://github.com/MetaCubeX/mihomo.git "$SRC"
  fi

  # Download module dependencies so we can patch sing-tun before building.
  (cd "$SRC" && go mod download)

  # Patch sing-tun: change the "fd not set" sentinel from ==0 to <0 and >0 to >=0.
  # ClashVpnService passes the TUN fd on fd 0 (stdin).  Without this patch,
  # sing-tun treats file-descriptor: 0 as "not set" and tries /dev/tun → EPERM.
  SINGTUN_VER=$(cd "$SRC" && go list -m github.com/metacubex/sing-tun | awk '{print $2}')
  GOMODCACHE=$(go env GOMODCACHE)
  SINGTUN_DIR="${GOMODCACHE}/github.com/metacubex/sing-tun@${SINGTUN_VER}"
  chmod -R u+w "$SINGTUN_DIR"
  PATCHED=$(find "$SINGTUN_DIR" -name "tun_linux.go" | xargs grep -l "FileDescriptor == 0" 2>/dev/null || true)
  if [ -n "$PATCHED" ]; then
    echo "$PATCHED" | xargs sed -i \
      -e 's/options\.FileDescriptor == 0/options.FileDescriptor < 0/g' \
      -e 's/\.FileDescriptor > 0/.FileDescriptor >= 0/g'
    echo "Patched sing-tun ${SINGTUN_VER}: ==0→<0 and >0→>=0"
  else
    echo "WARNING: sentinel not found in sing-tun ${SINGTUN_VER} — fd 0 TUN handoff may fail"
    grep -r "FileDescriptor" "$SINGTUN_DIR" --include="*.go" -l || true
  fi

  build_abi() {
    local ABI="$1" GOARCH="$2" GOARM="${3:-}"
    local DEST="$JNILIBS_ROOT/$ABI/libmihomo.so"
    mkdir -p "$JNILIBS_ROOT/$ABI"
    printf "%-16s GOOS=android GOARCH=%-6s -tags cmfa,with_gvisor … " "$ABI" "$GOARCH"
    GOOS=android GOARCH="$GOARCH" GOARM="$GOARM" CGO_ENABLED=0 \
      go build -C "$SRC" -tags cmfa,with_gvisor -trimpath -ldflags="-s -w" -o "$DEST" .
    echo "✓ $(du -h "$DEST" | cut -f1)"
  }

  build_abi arm64-v8a   arm64
  build_abi armeabi-v7a arm   7
  build_abi x86_64      amd64

  echo ""
  echo "Mihomo ${MIHOMO_VER} (GOOS=android -tags cmfa,with_gvisor + sing-tun fd-0 patch) ready for all ABIs."
  echo "Next: cd mobile && flutter build apk --debug"
  exit 0
fi

# ── Download pre-built binaries (emulator / CI only) ─────────────────────────
#
# WARNING: The downloaded binaries are suitable for CI E2E testing on the
# x86_64 emulator only.  The arm64-v8a binary produced here (linux-arm64)
# will fail on real devices with "configure tun interface: permission denied".
# For real-device builds use --build mode above, or use the release APK built
# by android-release.yml CI (which uses GOOS=android -tags cmfa).

echo "⚠️  Download mode: arm64-v8a binary will work on CI emulator but NOT real devices."
echo "    Use --build for real-device APKs."
echo ""

download_abi() {
  local ABI="$1" PRIMARY="$2" FALLBACK="${3:-}"
  local DEST="$JNILIBS_ROOT/$ABI/libmihomo.so"
  local TMP="/tmp/mihomo-${ABI}.gz"
  mkdir -p "$JNILIBS_ROOT/$ABI"

  local URL="https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VER}/mihomo-${PRIMARY}-${MIHOMO_VER}.gz"
  printf "%-16s %-34s … " "$ABI" "$PRIMARY"

  if curl -fsSL --retry 3 --retry-delay 2 -o "$TMP" "$URL" 2>/dev/null; then
    gunzip -f "$TMP"
    mv "${TMP%.gz}" "$DEST"
    chmod 644 "$DEST"
    echo "✓ $(du -h "$DEST" | cut -f1)"
    return 0
  fi

  if [ -n "$FALLBACK" ]; then
    local FURL="https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VER}/mihomo-${FALLBACK}-${MIHOMO_VER}.gz"
    printf "(fallback: %s) … " "$FALLBACK"
    if curl -fsSL --retry 3 --retry-delay 2 -o "$TMP" "$FURL" 2>/dev/null; then
      gunzip -f "$TMP"
      mv "${TMP%.gz}" "$DEST"
      chmod 644 "$DEST"
      echo "✓ $(du -h "$DEST" | cut -f1)"
      return 0
    fi
  fi

  echo "FAILED"
  return 1
}

FAIL=0

# arm64-v8a: linux build — OK for emulator, NOT for real devices (see warning above)
download_abi "arm64-v8a"   "linux-arm64"                  "" || FAIL=$((FAIL+1))
# x86_64: android build with linux-compatible fallback (matches CI emulator workflow)
download_abi "x86_64"      "android-amd64"                "linux-amd64-compatible" || FAIL=$((FAIL+1))
# armeabi-v7a: android build
download_abi "armeabi-v7a" "android-armv7"                "" || FAIL=$((FAIL+1))

echo ""
echo "Mihomo ${MIHOMO_VER} download complete (emulator/CI use only)."
[ "$FAIL" -eq 0 ] || { echo "⚠️  $FAIL ABI(s) failed — check network and retry."; exit 1; }
echo "Next: cd mobile && flutter build apk --debug"
