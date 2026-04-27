#!/bin/sh

case_03_stop_reset() {
  log "=== Case 03: stop + reset --start ==="
  run_remote_sh "stop exits takeover" "stop --yes" || return 1
  assert_vm_ok "mihomo stopped after stop" "i=0; while [ \$i -lt 10 ]; do ! pgrep -f mihomo-clashforge >/dev/null 2>&1 && exit 0; sleep 1; i=\$((i+1)); done; exit 1" || return 1

  run_remote_sh "reset --start succeeds" "reset --start --yes" || return 1
  assert_vm_ok "clashforge init script running" "/etc/init.d/clashforge status >/dev/null 2>&1" || return 1
  assert_vm_ok "API status reachable after reset --start" "wget -q -O - --timeout=8 http://127.0.0.1:7777/api/v1/status >/dev/null" || return 1
}
