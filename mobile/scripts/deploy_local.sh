#!/bin/sh
# Build + install a local test build of the mobile app onto a connected
# Android device, bumping pubspec.yaml's version first — mirrors
# scripts/clashforgectl.ps1's `deploy` action on the OpenWrt side (bump
# version -> build -> install), so a local test build is always visually
# distinguishable in the app's Settings/About screen from an official
# release pulled from GitHub, and successive local builds are distinguishable
# from each other too.
#
# Usage: mobile/scripts/deploy_local.sh [device-serial]
#   device-serial: optional adb device serial; defaults to the only
#   connected device (fails if zero or more than one is attached).

set -eu

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MOBILE_DIR="$REPO_ROOT/mobile"
PUBSPEC="$MOBILE_DIR/pubspec.yaml"
ADB="${ADB_PATH:-$LOCALAPPDATA/Android/Sdk/platform-tools/adb.exe}"
APP_ID="com.clashforge.mobile.clashforge_mobile"

log()  { printf '\033[36m[deploy_local]\033[0m %s\n' "$1"; }
die()  { printf '\033[31m[deploy_local] ERROR: %s\033[0m\n' "$1" >&2; exit 1; }

[ -f "$PUBSPEC" ] || die "pubspec.yaml not found: $PUBSPEC"
[ -x "$ADB" ] || die "adb not found at $ADB (set ADB_PATH to override)"

# ── Step 1: bump the local version ──────────────────────────────────────────
log "Step 1: bumping local version"

CURRENT=$(grep -m1 '^version:' "$PUBSPEC" | sed -E 's/^version:[[:space:]]*//')
[ -n "$CURRENT" ] || die "could not read version: line from $PUBSPEC"

# Expected shape: MAJOR.MINOR.PATCH[-PRERELEASE]+BUILD
CORE=$(echo "$CURRENT"   | sed -E 's/^([0-9]+\.[0-9]+\.[0-9]+).*/\1/')
PRE=$(echo "$CURRENT"    | sed -E 's/^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?\+[0-9]+$/\1/')
BUILD=$(echo "$CURRENT"  | sed -E 's/.*\+([0-9]+)$/\1/')
case "$CURRENT" in *+*) : ;; *) die "version must end in +BUILD (got: $CURRENT)";; esac

case "$PRE" in
  *.local) ;;                    # already marked local, leave prerelease as-is
  "") PRE="-local" ;;            # no prerelease segment at all
  *) PRE="${PRE}.local" ;;       # append the local marker once
esac

NEW_BUILD=$((BUILD + 1))
NEW_VERSION="${CORE}${PRE}+${NEW_BUILD}"

sed -i -E "s/^version:.*/version: ${NEW_VERSION}/" "$PUBSPEC"
log "version: ${CURRENT} -> ${NEW_VERSION}"

# ── Step 2: pick the target device ──────────────────────────────────────────
log "Step 2: resolving target device"

if [ $# -ge 1 ]; then
  DEVICE="$1"
else
  DEVICES=$("$ADB" devices | awk 'NR>1 && $2=="device" {print $1}')
  COUNT=$(echo "$DEVICES" | grep -c . || true)
  if [ "$COUNT" -eq 0 ]; then
    die "no authorized adb device attached (check USB debugging prompt on the phone)"
  elif [ "$COUNT" -gt 1 ]; then
    die "multiple devices attached, pass one explicitly: $DEVICES"
  fi
  DEVICE="$DEVICES"
fi
log "target device: $DEVICE"

# ── Step 3: build ────────────────────────────────────────────────────────────
log "Step 3: flutter build apk --debug"
( cd "$MOBILE_DIR" && flutter build apk --debug )

APK="$MOBILE_DIR/build/app/outputs/flutter-apk/app-debug.apk"
[ -f "$APK" ] || die "expected APK not found: $APK"

# ── Step 4: install + launch ────────────────────────────────────────────────
log "Step 4: installing to $DEVICE"
"$ADB" -s "$DEVICE" install -r "$APK"

log "Step 5: launching $APP_ID"
"$ADB" -s "$DEVICE" shell monkey -p "$APP_ID" -c android.intent.category.LAUNCHER 1 >/dev/null

log "Done — running version ${NEW_VERSION} on $DEVICE"
