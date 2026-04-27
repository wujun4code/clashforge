#!/bin/sh

case_05_uninstall_keep_config() {
  log "=== Case 05: uninstall --keep-config ==="

  [ -n "${E2E_IPK_PATH:-}" ] || {
    fail "E2E_IPK_PATH is required for uninstall recovery"
    return 1
  }

  assert_vm_ok "prepare metaclash marker" "mkdir -p /etc/metaclash && echo keep-me > /etc/metaclash/e2e-marker" || return 1
  run_ctl "uninstall --keep-config succeeds" uninstall --keep-config --yes || return 1

  assert_vm_ok "metaclash dir preserved" "[ -d /etc/metaclash ] && [ -f /etc/metaclash/e2e-marker ]" || return 1
  assert_vm_ok "package removed" "! opkg status clashforge 2>/dev/null | grep -q '^Package:'" || return 1

  # Reinstall so subsequent scenarios remain runnable
  vm_scp_to "$SCRIPT_SH" "/tmp/clashforgectl.sh" || return 1
  vm_scp_to "$E2E_IPK_PATH" "/tmp/e2e-reinstall.ipk" || return 1
  run_remote_sh "reinstall after uninstall" "upgrade --local-ipk /tmp/e2e-reinstall.ipk --yes" || return 1
  assert_vm_ok "API status reachable after reinstall" "wget -q -O - --timeout=10 http://127.0.0.1:7777/api/v1/status >/dev/null" || return 1
}
