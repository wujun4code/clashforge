#!/bin/sh

PASS_COUNT=0
FAIL_COUNT=0

log()  { printf '[clashforgectl-e2e] %s\n' "$*"; }
pass() { PASS_COUNT=$((PASS_COUNT+1)); printf '✅ %s\n' "$*"; }
fail() { FAIL_COUNT=$((FAIL_COUNT+1)); printf '❌ %s\n' "$*" >&2; }

vm_ssh() {
  ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 -p "$ROUTER_PORT" "$ROUTER_USER@$ROUTER_HOST" "$@"
}

vm_copy_to() {
  _local="$1"; _remote="$2"
  vm_ssh "cat > '$_remote'" < "$_local"
}

upload_remote_script() {
  _out=$(vm_ssh "cat > /tmp/clashforgectl.sh" < "$SCRIPT_SH" 2>&1)
  _code=$?
  if [ "$_code" -eq 0 ]; then
    pass "upload clashforgectl.sh to router"
  else
    fail "upload clashforgectl.sh to router"
    printf '%s\n' "$_out" >&2
  fi
  return "$_code"
}

run_remote_sh() {
  _name="$1"; shift
  _out=$(vm_ssh "sh /tmp/clashforgectl.sh $*" 2>&1)
  _code=$?
  if [ "$_code" -eq 0 ]; then
    pass "$_name"
  else
    fail "$_name (exit=$_code)"
    printf '%s\n' "$_out" >&2
  fi
  return "$_code"
}

assert_vm_ok() {
  _name="$1"; shift
  _out=$(vm_ssh "$*" 2>&1)
  _code=$?
  if [ "$_code" -eq 0 ]; then
    pass "$_name"
  else
    fail "$_name"
    printf '%s\n' "$_out" >&2
  fi
  return "$_code"
}

assert_vm_file_nonempty() {
  _name="$1"; _path="$2"
  assert_vm_ok "$_name" "[ -s '$_path' ]"
}

summary_and_exit() {
  log "SUMMARY: pass=$PASS_COUNT fail=$FAIL_COUNT"
  if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
  fi
}
