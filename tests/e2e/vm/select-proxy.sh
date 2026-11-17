#!/bin/sh
# VM-only: select the real AnyTLS route without changing rule/global mode.
# Discover the managed loopback controller port via CF; its runtime has no
# controller secret. Keep every config/proxy response private.
set -eu
set +x
umask 077
CF_API=${CF_API:-http://127.0.0.1:7777/api/v1}
NODE=${E2E_EXPECTED_NODE:-Tokyo-AnyTLS}
TYPE=${E2E_EXPECTED_TYPE:-anytls}
fail() { printf 'FAIL proxy selection: %s\n' "$1" >&2; exit 1; }
command -v jsonfilter >/dev/null 2>&1 || fail 'jsonfilter required'
WORK=$(mktemp -d "${TMPDIR:-/tmp}/cf-selection.XXXXXX")
trap 'rm -rf "$WORK"' EXIT HUP INT TERM
get() { curl --noproxy '*' -sf --connect-timeout 3 --max-time 10 "$CF_API/$1" > "$2" 2>/dev/null || fail 'API GET failed'; }
field() { jsonfilter -i "$1" -e "$2" 2>/dev/null || true; }
quote() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
key() { printf '@.data.proxies["%s"]%s' "$(quote "$1")" "$2"; }
urlencode() { printf '%s' "$1" | od -An -v -tx1 | awk '{for(i=1;i<=NF;i++) printf "%%%s",toupper($i)}'; }
has() { grep -Fxq -- "$1" "$2"; }
# Line-oriented graph storage cannot represent control bytes. Reject, never
# silently normalize names (emoji, spaces, percent signs and quotes are valid).
[ -n "$NODE" ] || fail 'empty expected node'
case "$NODE" in *'
'*) fail 'newline in node name' ;; esac
# Read the active controller, not YAML text: yaml.v3 may serialize astral
# group names as \U0001F41F, and raw awk cannot decode YAML scalars safely.
# Managed runtime strips the controller secret and binds it to loopback.
get config "$WORK/config.json"
[ "$(field "$WORK/config.json" '@.ok')" = true ] || fail 'config response rejected'
PORT=$(field "$WORK/config.json" '@.data.ports.mihomo_api')
case "$PORT" in ''|*[!0-9]*) fail 'invalid controller port' ;; esac
[ "$PORT" -gt 0 ] && [ "$PORT" -le 65535 ] || fail 'invalid controller port'
controller_get() {
    curl --noproxy '*' -sf --connect-timeout 3 --max-time 10 \
        "http://127.0.0.1:$PORT/$1" > "$2" 2>/dev/null || fail 'controller GET failed'
}
controller_get configs "$WORK/mode.json"
MODE=$(field "$WORK/mode.json" '@.mode')
[ "$MODE" = rule ] || fail 'expected rule mode; will not switch mode'
controller_get rules "$WORK/rules.json"
ROOT=$(field "$WORK/rules.json" '@.rules[@.type="Match"].proxy')
[ -n "$ROOT" ] || fail 'missing MATCH target in runtime rules'
case "$ROOT" in *'
'*|DIRECT|REJECT|GLOBAL) fail 'MATCH target is not a unique proxy group' ;; esac
get proxies "$WORK/before.json"
[ "$(field "$WORK/before.json" '@.ok')" = true ] || fail 'proxy response rejected'
ACTUAL_TYPE=$(field "$WORK/before.json" "$(key "$NODE" '.type')" | tr '[:upper:]' '[:lower:]')
[ "$ACTUAL_TYPE" = "$TYPE" ] && [ "$TYPE" = anytls ] || fail 'expected AnyTLS node missing or wrong protocol'
field "$WORK/before.json" '@.data.proxies[*].name' > "$WORK/names"
printf '%s\n' "$NODE" > "$WORK/reached"
: > "$WORK/plan"
# Bottom-up selector-only paths terminate at the verified node. Never select
# URLTest/DIRECT groups, and never let cycles establish reachability.
for pass in 1 2 3 4 5 6 7 8 9 10; do
    cp "$WORK/reached" "$WORK/previous"
    while IFS= read -r group; do
        [ -n "$group" ] && [ "$group" != GLOBAL ] || continue
        has "$group" "$WORK/reached" && continue
        [ "$(field "$WORK/before.json" "$(key "$group" '.type')")" = Selector ] || continue
        field "$WORK/before.json" "$(key "$group" '.all[*]')" > "$WORK/members"
        while IFS= read -r candidate; do
            if has "$candidate" "$WORK/reached"; then
                # Record JSON, not delimiter-separated names.
                printf '{"group":"%s","name":"%s"}\n' "$(quote "$group")" "$(quote "$candidate")" >> "$WORK/plan"
                printf '%s\n' "$group" >> "$WORK/reached"
                break
            fi
        done < "$WORK/members"
    done < "$WORK/names"
    cmp -s "$WORK/reached" "$WORK/previous" && break
done
has "$ROOT" "$WORK/reached" || fail 'MATCH selector chain cannot reach expected node'
[ -s "$WORK/plan" ] || fail 'no selectable path to expected node'
# Reachability can include unrelated selectors, including a dedicated DIRECT
# group that offers the main selector as an optional member. Only acquire the
# chosen MATCH path; do not change every reachable group in the graph.
current=$ROOT
: > "$WORK/needed"
while [ "$current" != "$NODE" ]; do
    has "$current" "$WORK/needed" && fail 'cycle in selection plan'
    printf '%s\n' "$current" >> "$WORK/needed"
    next=""
    while IFS= read -r row; do
        printf '%s\n' "$row" > "$WORK/row.json"
        if [ "$(field "$WORK/row.json" '@.group')" = "$current" ]; then
            next=$(field "$WORK/row.json" '@.name')
            break
        fi
    done < "$WORK/plan"
    [ -n "$next" ] || fail 'incomplete selection plan'
    current=$next
done
while IFS= read -r row; do
    printf '%s\n' "$row" > "$WORK/row.json"
    group=$(field "$WORK/row.json" '@.group')
    has "$group" "$WORK/needed" || continue
    choice=$(field "$WORK/row.json" '@.name')
    printf '{"name":"%s"}' "$(quote "$choice")" > "$WORK/payload.json"
    curl --noproxy '*' -sf --connect-timeout 3 --max-time 10 -X PUT \
        -H 'Content-Type: application/json' --data-binary "@$WORK/payload.json" \
        "$CF_API/proxies/$(urlencode "$group")/select" > "$WORK/put.json" 2>/dev/null || fail 'selector PUT failed'
    # Do not trust the wrapper's ok: empty upstream error bodies can be wrapped
    # as success. A separate GET must show the exact requested selector value.
    get proxies "$WORK/readback.json"
    [ "$(field "$WORK/readback.json" '@.ok')" = true ] || fail 'readback response rejected'
    [ "$(field "$WORK/readback.json" "$(key "$group" '.now')")" = "$choice" ] || fail 'stale selector readback'
done < "$WORK/plan"
get proxies "$WORK/final.json"
[ "$(field "$WORK/final.json" '@.ok')" = true ] || fail 'final response rejected'
current=$ROOT
: > "$WORK/visited"
while [ "$current" != "$NODE" ]; do
    has "$current" "$WORK/visited" && fail 'cycle in final selected chain'
    printf '%s\n' "$current" >> "$WORK/visited"
    [ "$(field "$WORK/final.json" "$(key "$current" '.type')")" = Selector ] || fail 'non-selector in final chain'
    next=$(field "$WORK/final.json" "$(key "$current" '.now')")
    field "$WORK/final.json" "$(key "$current" '.all[*]')" > "$WORK/members"
    [ -n "$next" ] && has "$next" "$WORK/members" || fail 'invalid final selected member'
    current=$next
done
[ "$(field "$WORK/final.json" "$(key "$NODE" '.type')" | tr '[:upper:]' '[:lower:]')" = anytls ] || fail 'final leaf protocol changed'
printf 'PASS proxy selection: rule MATCH chain read back to expected AnyTLS node\n'
