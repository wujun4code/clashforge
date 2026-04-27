#!/bin/sh
# tests/e2e/clashforgectl/run.sh
# Host-side E2E runner for scripts/clashforgectl against OpenWrt VM.

set -u

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)

ROUTER_HOST="${ROUTER_HOST:-127.0.0.1}"
ROUTER_PORT="${ROUTER_PORT:-2222}"
ROUTER_USER="${ROUTER_USER:-root}"
SCRIPT_SH="${SCRIPT_SH:-$REPO_ROOT/scripts/clashforgectl.sh}"
E2E_IPK_PATH="${E2E_IPK_PATH:-}"

. "$SCRIPT_DIR/assert.sh"
. "$SCRIPT_DIR/cases/01-status-check.sh"
. "$SCRIPT_DIR/cases/02-diag.sh"
. "$SCRIPT_DIR/cases/03-stop-reset.sh"
. "$SCRIPT_DIR/cases/04-upgrade-local-ipk.sh"
. "$SCRIPT_DIR/cases/05-uninstall-keep-config.sh"

log "Repo root     : $REPO_ROOT"
log "Router target : $ROUTER_USER@$ROUTER_HOST:$ROUTER_PORT"
log "Remote script : $SCRIPT_SH"
log "IPK path      : ${E2E_IPK_PATH:-<unset>}"

[ -f "$SCRIPT_SH" ] || { fail "missing script: $SCRIPT_SH"; summary_and_exit; }
[ -n "$E2E_IPK_PATH" ] || { fail "E2E_IPK_PATH is required"; summary_and_exit; }
[ -f "$E2E_IPK_PATH" ] || { fail "IPK not found: $E2E_IPK_PATH"; summary_and_exit; }

assert_vm_ok "router ssh reachable" "echo ok >/dev/null" || summary_and_exit
assert_vm_ok "clashforge init script exists" "[ -x /etc/init.d/clashforge ]" || summary_and_exit
upload_remote_script || summary_and_exit

case_01_status_check
case_02_diag
case_03_stop_reset
case_04_upgrade_local_ipk
case_05_uninstall_keep_config

summary_and_exit
