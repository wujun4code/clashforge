#!/usr/bin/env bash
# Keep all tool output private: Flutter may print encoded subscription secrets.
set +x
set -euo pipefail
umask 077
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
: "${GITHUB_WORKSPACE:?GITHUB_WORKSPACE is required}"
log="$RUNNER_TEMP/clashforge-e2e-flutter.txt"
: > "$log"
chmod 600 "$log"
# Keep freshness and result checks outside the child shell's script stdin.
apk="$GITHUB_WORKSPACE/mobile/build/app/outputs/flutter-apk/app-debug.apk"
rm -f "$apk" >>"$log" 2>&1
# A separate bash preserves errexit even though the parent records its status.
set +e
bash -euo pipefail -s >"$log" 2>&1 <<'BUILD'
flutter_sdk="$(dirname "$(dirname "$(readlink -f "$(command -v flutter)")")")"
# Read the effective SDK extension, rather than guessing from installed NDKs.
ndk_version="$(python3 - "$flutter_sdk/packages/flutter_tools/gradle/src/main/kotlin/FlutterExtension.kt" <<'PY'
import pathlib, re, sys
text = pathlib.Path(sys.argv[1]).read_text()
versions = re.findall(r'val\s+ndkVersion\s*:\s*String\s*=\s*"([0-9.]+)"', text)
if versions != ['28.2.13676358']:
    raise SystemExit('Unexpected pinned Flutter NDK version')
print(versions[0])
PY
)"
sdk="${ANDROID_HOME:-${ANDROID_SDK_ROOT:?Android SDK path is required}}"
properties="$sdk/ndk/$ndk_version/source.properties"
valid_ndk() {
  python3 - "$properties" "$ndk_version" <<'PY'
import pathlib, re, sys
p = pathlib.Path(sys.argv[1])
if not p.is_file():
    raise SystemExit(1)
versions = re.findall(r'^\s*Pkg\.Revision\s*=\s*(\S+)\s*$', p.read_text(), re.M)
raise SystemExit(0 if versions == [sys.argv[2]] else 1)
PY
}
if ! valid_ndk; then
  sdkmanager="$sdk/cmdline-tools/latest/bin/sdkmanager"
  if [ ! -x "$sdkmanager" ]; then sdkmanager="$(command -v sdkmanager)"; fi
  installed=false
  for attempt in 1 2; do
    # Official installer, default TLS verification; bound both retries and time.
    if timeout --kill-after=10s 300 "$sdkmanager" --sdk_root="$sdk" "ndk;$ndk_version" </dev/null; then
      if valid_ndk; then installed=true; break; fi
    fi
  done
  # A failed installer may leave matching metadata; that is not success.
  "$installed"
fi
cd "$GITHUB_WORKSPACE/mobile"
flutter build apk --debug "--dart-define=SUBSCRIPTION_URL=${SUBSCRIPTION_URL:?SUBSCRIPTION_URL is required}" </dev/null
BUILD
status=$?
set -e
# Never replace a nonzero Flutter/installer exit with an APK-check result.
if [ "$status" -eq 0 ] && ! test -s "$apk"; then status=1; fi
if [ "$status" -ne 0 ]; then
  echo 'APK pre-build failed; raw diagnostics retained privately for encryption.' >&2
  exit "$status"
fi
echo 'APK pre-build completed; expected nonempty debug APK verified.'
