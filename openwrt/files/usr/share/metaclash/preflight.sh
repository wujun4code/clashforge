#!/bin/sh
# Shared install/start prerequisites, NOT a TUNSETIFF or data-plane health test.
# Never source TOML as shell or replace an explicitly selected core.

cf_preflight_error() {
    echo "ClashForge preflight: $*" >&2
    logger -t clashforge "ERROR: $*"
    return 1
}

cf_config_string() {
    # Deliberately bounded TOML subset: ordinary table headers and unescaped
    # basic/literal strings. Refuse ambiguous syntax rather than guess a mode
    # or binary. Full TOML validation remains the daemon's responsibility.
    awk -v section="$2" -v key="$3" -v fallback="$4" '
    function trim(s) { sub(/^[ \t]+/, "", s); sub(/[ \t\r]+$/, "", s); return s }
    BEGIN { sq = sprintf("%c", 39) }
    {
        line = trim($0)
        if (line ~ /^[#]/ || line == "") next
        # Quoted/dotted keys are valid TOML but outside this parser subset.
        # Never let a quoted binary/mode silently select a different default.
        if (substr(line, 1, 1) == "\"" || substr(line, 1, 1) == sq) { bad = 1; next }
        if (line ~ /^(core|network)[ \t]*[.=]/) { bad = 1; next }
        if (line ~ /^\[/) {
            sub(/[ \t]*#.*$/, "", line)
            if (line !~ /^\[[A-Za-z0-9_.-]+\]$/) { bad = 1; next }
            current = substr(line, 2, length(line)-2)
            next
        }
        if (current != section || line !~ ("^" key "[ \t]*=")) next
        if (found++) { bad = 1; next }
        sub(/^[^=]*=[ \t]*/, "", line)
        quote = substr(line, 1, 1)
        if (quote != "\"" && quote != sq) { bad = 1; next }
        rest = substr(line, 2)
        end = index(rest, quote)
        if (!end) { bad = 1; next }
        value = substr(rest, 1, end-1)
        tail = trim(substr(rest, end+1))
        if (tail != "" && tail !~ /^#/) bad = 1
        if (quote == "\"" && index(value, "\\")) bad = 1
    }
    END {
        if (bad) exit 2
        if (found) print value; else print fallback
    }' "$1"
}

cf_preflight() {
    local config="$1" mode bin version rc stack driver
    [ -r "$config" ] || { cf_preflight_error "cannot read $config"; return 1; }
    mode=$(cf_config_string "$config" network mode tproxy) || {
        cf_preflight_error "unsupported/ambiguous TOML; use ordinary [network] mode string"; return 1;
    }
    bin=$(cf_config_string "$config" core binary /usr/bin/mihomo) || {
        cf_preflight_error "unsupported/ambiguous [core] binary string"; return 1;
    }
    [ -n "$bin" ] && [ -x "$bin" ] && [ ! -d "$bin" ] || {
        cf_preflight_error "selected core is not executable: $bin (configuration preserved)"; return 1;
    }
    rc=0
    if command -v timeout >/dev/null 2>&1; then
        version=$(timeout 5 "$bin" -v 2>&1) || rc=$?
    elif [ -x /usr/bin/clashforge ]; then
        # Some OpenWrt BusyBox builds omit timeout. The daemon helper uses a
        # Go context deadline and exits before config loading or service setup.
        version=$(/usr/bin/clashforge -core-version-check "$bin" 2>&1) || rc=$?
    else
        cf_preflight_error "bounded version check unavailable: require timeout or the installed ClashForge helper"; return 1
    fi
    case "$rc" in
        0|1) ;; # Some Mihomo versions return 1 for -v, despite valid output.
        *) cf_preflight_error "selected core version check failed (exit $rc): $bin"; return 1 ;;
    esac
    printf '%s\n' "$version" | grep -qE '^Mihomo Meta v?[0-9]+\.[0-9]+\.[0-9]+([[:space:]]|$)' || {
        cf_preflight_error "unrecognized Mihomo version from selected core: $bin"; return 1;
    }
    logger -t clashforge "Selected core: $bin; $(printf '%s\n' "$version" | head -1)"
    case "$mode" in
        tproxy|tun)
            # BusyBox hides protocol ownership and cannot select rule protocol
            # on delete. Never fall back to its lossy textual rule/route output.
            ip -N -d -o rule show >/dev/null 2>&1 || {
                cf_preflight_error "policy routing requires iproute2 protocol ownership support: install ip-full (opkg update && opkg install ip-full); BusyBox ip is unsupported"; return 1;
            } ;;
    esac
    [ "$mode" = tun ] || return 0

    stack=$(cf_config_string "$config" network.tun stack mixed) || {
        cf_preflight_error "unsupported/ambiguous [network.tun] stack"; return 1;
    }
    case "$stack" in
        mixed|gvisor)
            printf '%s\n' "$version" | grep -qE '(^|[^A-Za-z0-9_])with_gvisor([^A-Za-z0-9_]|$)' || {
                cf_preflight_error "selected core $bin lacks with_gvisor for TUN stack=$stack"; return 1;
            } ;;
        system) ;;
        *) cf_preflight_error "unsupported TUN stack: $stack"; return 1 ;;
    esac
    [ "$(id -u)" = 0 ] || {
        cf_preflight_error "TUN service requires root/CAP_NET_ADMIN; start via procd"; return 1;
    }
    driver=/sys/class/misc/tun/dev
    if [ "$(cat "$driver" 2>/dev/null)" != 10:200 ]; then
        modprobe tun 2>/dev/null || true
    fi
    [ "$(cat "$driver" 2>/dev/null)" = 10:200 ] || {
        cf_preflight_error "kernel TUN driver unavailable; install firmware-matched kmod-tun/CONFIG_TUN"; return 1;
    }
    # Do not manufacture a device node when no registered driver exists.
    [ -c /dev/net/tun ] && [ -r /dev/net/tun ] && [ -w /dev/net/tun ] &&
        [ "$(stat -c '%t:%T' /dev/net/tun 2>/dev/null)" = a:c8 ] || {
        cf_preflight_error "missing/inaccessible TUN character device 10:200 at /dev/net/tun"; return 1;
    }
    ip -4 rule show >/dev/null 2>&1 && ip -4 route show table main >/dev/null 2>&1 || {
        cf_preflight_error "TUN requires working ip rule/route support (ip-full)"; return 1;
    }
    logger -t clashforge "TUN prerequisites checked; interface creation, CAP_NET_ADMIN and LAN/DNS forwarding still require runtime verification"
}
