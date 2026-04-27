#!/bin/sh

case_03_stop_reset() {
  log "=== Case 03: stop + reset --start ==="
  run_remote_sh "stop exits takeover" "stop --yes" || return 1
  assert_vm_ok "takeover nft table removed after stop" "! nft list table inet metaclash >/dev/null 2>&1" || return 1
  assert_vm_ok "policy route mark cleared after stop" "! ip rule list 2>/dev/null | grep -q '0x1a3'" || return 1

  run_remote_sh "reset --start succeeds" "reset --start --yes" || return 1
  assert_vm_ok "clashforge init script running" "/etc/init.d/clashforge status >/dev/null 2>&1" || return 1
  assert_vm_ok "API status reachable after reset --start" "wget -q -O - --timeout=8 http://127.0.0.1:7777/api/v1/status >/dev/null" || return 1
}
