#!/usr/bin/env bash
set -euo pipefail

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
adb reverse tcp:9050 tcp:9050
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
    --timeout=540s 2>&1 |
    tee "$FLUTTER_LOG_FILE"
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
else
  printf '{"ok":false,"baseline_ip":"%s","vpn_ip":"","detail":"Flutter E2E readiness marker was not observed"}\n' \
    "$BASELINE_IP" > "$RESULT_LOCAL"
fi

adb shell run-as "$PKG" sh -c "cat > $RESULT_REMOTE" < "$RESULT_LOCAL"

set +e
wait "$FLUTTER_PID"
TEST_RC=$?
set -e

sleep 2
echo "=== MihomoCore + ClashVpn + E2E Probe logcat ==="
grep -E "MihomoCore|ClashVpn|E2E|TUN|gvisor|mihomo|PR-01|PR-02|PASS|FAIL|WARN|CONNECTIVITY|proxy|flutter|CF_IMPORT" "$LOGCAT_FILE" 2>/dev/null | tail -300 || true
exit "$TEST_RC"
