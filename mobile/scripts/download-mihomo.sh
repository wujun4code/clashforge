#!/usr/bin/env bash
# Download mihomo for Android ABIs and place as jniLibs so the app can bundle it.
#
# PINNED to v1.18.10:
#   v1.19.x reads /data/system/packages.xml unconditionally during TUN init
#   ("build android rules"), which is denied by SELinux on non-rooted devices
#   and on API 29+ emulators without explicit chmod.  v1.18.10 does NOT perform
#   this read, so TUN initializes correctly with auto-route=false.
#
# Usage (from repo root):
#   bash mobile/scripts/download-mihomo.sh
#
# Re-run whenever you bump MIHOMO_VER.

set -euo pipefail

MIHOMO_VER="v1.18.10"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JNILIBS_ROOT="$SCRIPT_DIR/../android/app/src/main/jniLibs"

# ABI → mihomo release arch suffix
declare -A ABI_ARCH=(
  ["arm64-v8a"]="android-arm64"
  ["x86_64"]="android-amd64"
  ["armeabi-v7a"]="android-armv7"
)

FAIL=0
for ABI in "${!ABI_ARCH[@]}"; do
  ARCH="${ABI_ARCH[$ABI]}"
  DEST="$JNILIBS_ROOT/$ABI/libmihomo.so"
  mkdir -p "$JNILIBS_ROOT/$ABI"

  URL="https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VER}/mihomo-${ARCH}-${MIHOMO_VER}.gz"
  TMP="/tmp/mihomo-${ABI}.gz"

  printf "%-16s %s … " "$ABI" "$ARCH"
  if curl -fsSL --retry 3 --retry-delay 2 -o "$TMP" "$URL" 2>/dev/null; then
    gunzip -f "$TMP"
    mv "${TMP%.gz}" "$DEST"
    chmod 644 "$DEST"
    SIZE=$(du -h "$DEST" | cut -f1)
    echo "✓ $SIZE → $DEST"
  else
    # x86_64 fallback: linux-amd64-compatible static binary works on emulator
    if [ "$ABI" = "x86_64" ]; then
      FALLBACK_URL="https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VER}/mihomo-linux-amd64-compatible-${MIHOMO_VER}.gz"
      printf "(fallback) "
      if curl -fsSL --retry 3 --retry-delay 2 -o "$TMP" "$FALLBACK_URL" 2>/dev/null; then
        gunzip -f "$TMP"
        mv "${TMP%.gz}" "$DEST"
        chmod 644 "$DEST"
        SIZE=$(du -h "$DEST" | cut -f1)
        echo "✓ $SIZE → $DEST"
        continue
      fi
    fi
    echo "FAILED (skipped)"
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "Mihomo ${MIHOMO_VER} download complete."
if [ "$FAIL" -gt 0 ]; then
  echo "⚠️  $FAIL ABI(s) failed — check network and retry."
  exit 1
fi

echo "Next step:  flutter build apk  (from mobile/)"
