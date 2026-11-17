#!/bin/sh
# Read-only TUN-specific cleanup assertions. No route/rule/firewall mutations.
set -eu
rules=$(ip -4 -d rule show) || { echo 'FAIL: cannot inspect IPv4 rules' >&2; exit 1; }
if printf '%s\n' "$rules" | grep -E '^902[01]:' | grep -qE '(proto|protocol) 242([[:space:]]|$)'; then
    echo 'FAIL: owned TUN policy rules remain after stop' >&2
    exit 1
fi
forward=$(nft -a list chain inet fw4 forward_lan) || {
    echo 'FAIL: cannot inspect fw4 forward_lan after stop' >&2; exit 1;
}
if printf '%s\n' "$forward" | grep -qF 'comment "clashforge: allow forwarding into TUN device"'; then
    echo 'FAIL: ClashForge TUN forward rule remains after stop' >&2
    exit 1
fi
# Absence of the owned rules does not prove baseline LAN/DNS connectivity.
echo 'PASS: no ClashForge-owned TUN policy/forward rules remain (control plane only)'
