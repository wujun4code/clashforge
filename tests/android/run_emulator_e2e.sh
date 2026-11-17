#!/usr/bin/env bash
set -euo pipefail
umask 077 # Raw local diagnostics may contain subscription endpoints.

PKG="com.clashforge.mobile.clashforge_mobile"
RESULT_REMOTE="/data/data/${PKG}/files/e2e_tun_probe_result.json"
RESULT_LOCAL="${RUNNER_TEMP:-/tmp}/clashforge-e2e-tun-result.json"
LOGCAT_FILE="${RUNNER_TEMP:-/tmp}/clashforge-e2e-logcat.txt"
FLUTTER_LOG_FILE="${RUNNER_TEMP:-/tmp}/clashforge-e2e-flutter.txt"
PROBE_BIN="${GITHUB_WORKSPACE}/tests/android/tun_ip_probe_android_amd64"

run_device_probe() {
  local out_file rc
  out_file="$(mktemp)"
  set +e
  adb shell /data/local/tmp/cf-tun-probe >"$out_file" 2>&1
  rc=$?
  set -e
  tr -d '\r' < "$out_file" >&2
  if [ "$rc" -ne 0 ]; then
    rm -f "$out_file"
    return "$rc"
  fi
  tr -d '\r' < "$out_file" | tail -1
  rm -f "$out_file"
}

cleanup() {
  for pid in "${BGPID:-}" "${CHMOD_PID:-}" "${LOGCAT_PID:-}" "${FLUTTER_PID:-}"; do
    if [ -n "$pid" ]; then
      kill "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT

adb root 2>/dev/null || true
adb wait-for-device
adb shell setenforce 0 2>/dev/null || true
adb shell chmod o+r /data/system/packages.xml 2>/dev/null || true
adb install -t -r "${GITHUB_WORKSPACE}/mobile/build/app/outputs/flutter-apk/app-debug.apk"
adb push "$PROBE_BIN" /data/local/tmp/cf-tun-probe >/dev/null
adb shell chmod 755 /data/local/tmp/cf-tun-probe
adb shell run-as "$PKG" rm -f "$RESULT_REMOTE" 2>/dev/null || true
# Remove opt-in synthetic Tor state from reused emulators. This E2E must use
# the imported subscription, not the application's separate CI transport hook.
adb reverse --remove tcp:9050 2>/dev/null || true
adb shell rm -f /data/local/tmp/clashforge_ci_tor.enable
adb logcat -c 2>/dev/null || true

echo "Running adb-shell baseline IP probe before VPN starts..."
BASELINE_IP="$(run_device_probe)"
if [ -z "$BASELINE_IP" ]; then
  echo "ERROR: adb-shell baseline IP probe returned empty output"
  exit 1
fi
echo "adb-shell baseline IP: $BASELINE_IP"

adb logcat -v time > "$LOGCAT_FILE" 2>&1 &
LOGCAT_PID=$!
(while true; do adb shell chmod o+r /data/system/packages.xml 2>/dev/null; sleep 1; done) &
CHMOD_PID=$!
(while true; do
  if adb shell uiautomator dump /sdcard/d.xml >/dev/null 2>&1 &&
     adb pull /sdcard/d.xml /tmp/vd.xml >/dev/null 2>&1 &&
     grep -q vpndialogs /tmp/vd.xml 2>/dev/null; then
    python3 -c "import xml.etree.ElementTree as ET,re; [print((int(m.group(1))+int(m.group(3)))//2,(int(m.group(2))+int(m.group(4)))//2) for n in ET.parse('/tmp/vd.xml').iter() if n.get('text') in ('OK','Allow','ALLOW') for m in [re.match(r'\[(\d+),(\d+)\]\[(\d+),(\d+)\]',n.get('bounds',''))] if m]" 2>/dev/null |
      head -1 |
      xargs -r adb shell input tap >/dev/null 2>&1
  fi
  sleep 1
done) &
BGPID=$!

cd "${GITHUB_WORKSPACE}/mobile"
(
  timeout 900 flutter test integration_test/app_e2e_test.dart \
    --device-id emulator-5554 \
    "--dart-define=SUBSCRIPTION_URL=${SUBSCRIPTION_URL}" \
    "--dart-define=E2E_REQUIRE_EXTERNAL_TUN_PROBE=true" \
    --reporter=expanded \
    --timeout=540s > "$FLUTTER_LOG_FILE" 2>&1
  # Flutter output can include imported-node errors. Keep it local and encrypt
  # it for private diagnosis; do not tee raw stdout/stderr to the CI console.
) &
FLUTTER_PID=$!

echo "Waiting for Flutter E2E readiness marker..."
READY=0
for _ in $(seq 1 600); do
  if grep -q "E2E_READY_FOR_EXTERNAL_TUN_PROBE" "$LOGCAT_FILE" "$FLUTTER_LOG_FILE" 2>/dev/null; then
    READY=1
    break
  fi
  if ! kill -0 "$FLUTTER_PID" 2>/dev/null; then
    break
  fi
  sleep 1
done

RESULT_RC=0
if [ "$READY" -eq 1 ]; then
  echo "Running adb-shell IP probe while VPN is connected..."
  VPN_IP="$(run_device_probe || true)"
  OK=false
  DETAIL="adb-shell VPN IP must be non-empty and differ from baseline"
  if [ -n "$VPN_IP" ] && [ "$VPN_IP" != "$BASELINE_IP" ]; then
    OK=true
    DETAIL="adb-shell traffic changed exit IP through VPN/TUN"
  fi
  printf '{"ok":%s,"baseline_ip":"%s","vpn_ip":"%s","detail":"%s"}\n' \
    "$OK" "$BASELINE_IP" "$VPN_IP" "$DETAIL" > "$RESULT_LOCAL"
  # adb concatenates shell arguments: quote the remote sh -c payload so its
  # redirection runs *inside* run-as, with the app's data directory and UID.
  adb shell "run-as $PKG sh -c 'cat > files/e2e_tun_probe_result.json'" < "$RESULT_LOCAL" || RESULT_RC=$?
else
  # Flutter tears the test app down on failure. Do not write a result into its
  # now-absent data directory and mask the real Flutter exit status.
  RESULT_RC=1 # Even Flutter exit 0 cannot prove an external TUN probe ran.
  echo "Flutter E2E readiness marker was not observed; preserving Flutter failure"
fi

set +e
wait "$FLUTTER_PID"
TEST_RC=$?
set -e

sleep 2
echo "=== Private core log classification (counts only) ==="
# Core logs can contain real subscription endpoints. Keep raw logcat local;
# never print it or upload it as an unrestricted CI artifact.
python3 - "$LOGCAT_FILE" <<'PY'
import json
from pathlib import Path
import sys
text = Path(sys.argv[1]).read_text(errors='replace').lower()
patterns = {
    'certificate_error': ('x509:', 'certificate signed by unknown authority', 'certificate is valid for'),
    'dial_timeout': ('i/o timeout', 'context deadline exceeded'),
    'connection_refused': ('connection refused',),
    'tls_error': ('tls handshake', 'handshake failure', 'handshake error'),
    'core_exit': ('core exited',),
    'app_excluded': ('excluded app from vpn routing',),
}
print(json.dumps({key: sum(text.count(p) for p in values)
                  for key, values in patterns.items()}, sort_keys=True))
PY
if [ "$TEST_RC" -ne 0 ]; then
  exit "$TEST_RC"
fi
exit "$RESULT_RC"
