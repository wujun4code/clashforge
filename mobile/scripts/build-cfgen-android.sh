#!/usr/bin/env bash
# Build libcfgen.so — the Android JNI config-generation library.
#
# The library exposes two JNI functions called by ClashVpnService:
#   nativeProbeAndPatchDNS  — pre-VPN upstream DNS hijack probe
#   nativeGenerateConfig    — core-apply: subscription + options → final YAML
#
# Usage (from repo root):
#   bash mobile/scripts/build-cfgen-android.sh
#   bash mobile/scripts/build-cfgen-android.sh --abi arm64-v8a   # default
#   bash mobile/scripts/build-cfgen-android.sh --abi x86_64      # emulator
#
# Requires: Go (same version used for mihomo), Android NDK.
# NDK is auto-detected from ANDROID_NDK_HOME, ANDROID_HOME, or flutter doctor.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CFGEN_DIR="$SCRIPT_DIR/../cfgen"
JNILIBS_ROOT="$SCRIPT_DIR/../android/app/src/main/jniLibs"
ABI="${2:-arm64-v8a}"
MIN_API=21

# ── ABI → Go arch mapping ────────────────────────────────────────────────────
case "$ABI" in
  arm64-v8a)   GOARCH=arm64  ;;
  armeabi-v7a) GOARCH=arm    ; GOARM=7 ;;
  x86_64)      GOARCH=amd64  ;;
  x86)         GOARCH=386    ;;
  *)           echo "Unknown ABI: $ABI"; exit 1 ;;
esac

# ── Locate Android NDK ───────────────────────────────────────────────────────
find_ndk() {
  # 1. Explicit env var
  if [ -n "${ANDROID_NDK_HOME:-}" ] && [ -d "$ANDROID_NDK_HOME" ]; then
    echo "$ANDROID_NDK_HOME"; return
  fi
  # 2. ANDROID_HOME/ndk/<version> (highest version wins)
  if [ -n "${ANDROID_HOME:-}" ] && [ -d "$ANDROID_HOME/ndk" ]; then
    local best
    best=$(ls -1 "$ANDROID_HOME/ndk" 2>/dev/null | sort -V | tail -1)
    if [ -n "$best" ]; then echo "$ANDROID_HOME/ndk/$best"; return; fi
  fi
  # 3. Common Windows path via Git Bash
  local win_sdk="$LOCALAPPDATA/Android/Sdk"
  if [ -d "$win_sdk/ndk" ]; then
    local best
    best=$(ls -1 "$win_sdk/ndk" 2>/dev/null | sort -V | tail -1)
    if [ -n "$best" ]; then echo "$win_sdk/ndk/$best"; return; fi
  fi
  # 4. macOS / Linux default
  local home_sdk="$HOME/Library/Android/sdk"
  if [ -d "$home_sdk/ndk" ]; then
    local best
    best=$(ls -1 "$home_sdk/ndk" 2>/dev/null | sort -V | tail -1)
    if [ -n "$best" ]; then echo "$home_sdk/ndk/$best"; return; fi
  fi
  echo ""
}

NDK="$(find_ndk)"
if [ -z "$NDK" ]; then
  echo "ERROR: Android NDK not found."
  echo "Set ANDROID_NDK_HOME or ANDROID_HOME and retry."
  exit 1
fi
echo "NDK: $NDK"

# ── Resolve NDK host tag ─────────────────────────────────────────────────────
case "$(uname -s)" in
  Linux)   HOST_TAG=linux-x86_64   ;;
  Darwin)  HOST_TAG=darwin-x86_64  ;;
  MINGW*|MSYS*|CYGWIN*) HOST_TAG=windows-x86_64 ;;
  *)       HOST_TAG=linux-x86_64   ;;
esac
TOOLCHAIN="$NDK/toolchains/llvm/prebuilt/$HOST_TAG/bin"

# ── Select clang for the target ABI ─────────────────────────────────────────
case "$ABI" in
  arm64-v8a)   CLANG_PREFIX="aarch64-linux-android${MIN_API}"  ;;
  armeabi-v7a) CLANG_PREFIX="armv7a-linux-androideabi${MIN_API}" ;;
  x86_64)      CLANG_PREFIX="x86_64-linux-android${MIN_API}"   ;;
  x86)         CLANG_PREFIX="i686-linux-android${MIN_API}"      ;;
esac

# Windows NDK uses .cmd wrappers; Linux/macOS use bare executables.
if [ -f "$TOOLCHAIN/${CLANG_PREFIX}-clang.cmd" ]; then
  CC="$TOOLCHAIN/${CLANG_PREFIX}-clang.cmd"
elif [ -f "$TOOLCHAIN/${CLANG_PREFIX}-clang" ]; then
  CC="$TOOLCHAIN/${CLANG_PREFIX}-clang"
else
  echo "ERROR: clang not found at $TOOLCHAIN/${CLANG_PREFIX}-clang[.cmd]"
  echo "NDK version may be too old (need r21+) or ABI unsupported."
  exit 1
fi
echo "CC:  $CC"

# ── Build ────────────────────────────────────────────────────────────────────
DEST="$JNILIBS_ROOT/$ABI/libcfgen.so"
mkdir -p "$(dirname "$DEST")"

echo ""
echo "Building libcfgen.so for $ABI (GOOS=android GOARCH=$GOARCH)…"

(
  cd "$CFGEN_DIR"
  export CGO_ENABLED=1
  export GOOS=android
  export GOARCH="$GOARCH"
  export CC="$CC"
  ${GOARM:+export GOARM="$GOARM"}

  go build \
    -buildmode=c-shared \
    -trimpath \
    -ldflags="-s -w" \
    -o "$DEST" \
    ./cmd/android/
)

echo ""
echo "✓ $(du -h "$DEST" | cut -f1)  $DEST"
echo ""
echo "Next steps:"
echo "  cd mobile && flutter build apk --debug"
