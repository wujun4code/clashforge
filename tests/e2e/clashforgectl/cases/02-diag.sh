#!/bin/sh

case_02_diag() {
  log "=== Case 02: diag ==="
  _remote="/tmp/cf-diag-e2e.txt"
  run_remote_sh "diag writes report" "diag --output $_remote" || return 1
  assert_vm_file_nonempty "diag report exists" "$_remote" || return 1
  assert_vm_ok "diag report includes process section" "grep -q 'Process state' '$_remote'" || return 1
}
