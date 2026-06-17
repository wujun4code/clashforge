#!/bin/sh

case_04_upgrade_local_ipk() {
  log "=== Case 04: upgrade --local-ipk ==="

  [ -n "${E2E_IPK_PATH:-}" ] || {
    fail "E2E_IPK_PATH is required for local upgrade case"
    return 1
  }
  [ -f "$E2E_IPK_PATH" ] || {
    fail "E2E_IPK_PATH not found: $E2E_IPK_PATH"
    return 1
  }

  vm_copy_to "$SCRIPT_SH" "/tmp/clashforgectl.sh" || return 1
  vm_copy_to "$E2E_IPK_PATH" "/tmp/e2e-upgrade.ipk" || return 1

  run_remote_sh "remote upgrade --local-ipk" "upgrade --local-ipk /tmp/e2e-upgrade.ipk --yes" || return 1
  assert_api_ready "API status reachable after local upgrade" || return 1
}
