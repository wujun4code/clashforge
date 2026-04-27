#!/bin/sh

case_01_status_check() {
  log "=== Case 01: status/check ==="
  run_ctl "status returns 0" status || return 1
  run_ctl "check returns 0" check || return 1

  # API should stay reachable after read-only commands
  assert_vm_ok "API status reachable" "wget -q -O - --timeout=5 http://127.0.0.1:7777/api/v1/status >/dev/null" || return 1
}
