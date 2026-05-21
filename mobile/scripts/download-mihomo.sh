#!/usr/bin/env bash
# Download mihomo for Android ABIs and place as jniLibs so the app can bundle it.
#
# PINNED to v1.18.10, using the LINUX build for arm64-v8a:
#
#   Why linux-arm64 instead of android-arm64-v8?
#   The android build compiles with GOOS=android which adds a platform check:
#   "if running on Android → buildAndroidRules() → read /data/system/packages.xml".
#   On non-rooted devices SELinux blocks this read and TUN init aborts entirely.
#   The linux build (GOOS=linux) has no such branch.  Since the binary is a pure-Go
#   static binary (CGO disabled) it runs on Android's Linux kernel without any libc
#   dependency.  We provide our own TUN fd via /proc/self/fd/N and handle routing via
#   Android VPNService, so linux-mode TUN init works perfectly.
#
#   x86_64 (emulator): keep android-amd64 with linux-amd64-compatible fallback,
#   matching the CI workflow which runs chmod o+r packages.xml as workaround.
#
# Usage (from repo root):
#   bash mobile/scripts/download-mihomo.sh
#
# Windows alternative (PowerShell, from repo root):
#   See the comment block at the bottom of this file.

set -euo pipefail

MIHOMO_VER="v1.18.10"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
JNILIBS_ROOT="$SCRIPT_DIR/../android/app/src/main/jniLibs"

# ABI → (primary URL suffix, fallback URL suffix or "")
# arm64-v8a uses linux build intentionally — see header comment.
download_abi() {
  local ABI="$1"
  local PRIMARY="$2"
  local FALLBACK="${3:-}"

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

# arm64-v8a: linux build avoids android-specific packages.xml read
download_abi "arm64-v8a"   "linux-arm64"                  "" || FAIL=$((FAIL+1))

# x86_64: android build with linux-compatible fallback (matches CI workflow)
download_abi "x86_64"      "android-amd64"                "linux-amd64-compatible" || FAIL=$((FAIL+1))

# armeabi-v7a: android build (correct v8 suffix not present for armv7)
download_abi "armeabi-v7a" "android-armv7"                "" || FAIL=$((FAIL+1))

echo ""
echo "Mihomo ${MIHOMO_VER} binaries ready."
[ "$FAIL" -eq 0 ] || { echo "⚠️  $FAIL ABI(s) failed — check network and retry."; exit 1; }
echo "Next: cd mobile && flutter build apk --debug"

# ── Windows PowerShell equivalent ──────────────────────────────────────────────
# If you're on Windows without WSL, run these commands in PowerShell from the repo root:
#
#   $ver = "v1.18.10"
#   $base = "https://github.com/MetaCubeX/mihomo/releases/download/$ver"
#
#   # arm64-v8a (linux build — no packages.xml read on real devices)
#   New-Item -ItemType Directory -Force "mobile\android\app\src\main\jniLibs\arm64-v8a" | Out-Null
#   Invoke-WebRequest "$base/mihomo-linux-arm64-$ver.gz" -OutFile "$env:TEMP\mihomo.gz"
#   & wsl gunzip -f /mnt/c/Users/$env:USERNAME/AppData/Local/Temp/mihomo.gz
#   Move-Item "$env:TEMP\mihomo" "mobile\android\app\src\main\jniLibs\arm64-v8a\libmihomo.so" -Force
#
#   # x86_64 (android build — for emulator, matches CI)
#   New-Item -ItemType Directory -Force "mobile\android\app\src\main\jniLibs\x86_64" | Out-Null
#   Invoke-WebRequest "$base/mihomo-android-amd64-$ver.gz" -OutFile "$env:TEMP\mihomo.gz"
#   & wsl gunzip -f /mnt/c/Users/$env:USERNAME/AppData/Local/Temp/mihomo.gz
#   Move-Item "$env:TEMP\mihomo" "mobile\android\app\src\main\jniLibs\x86_64\libmihomo.so" -Force
