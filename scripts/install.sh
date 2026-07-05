#!/bin/sh
# ClashForge remote-management CLI installer (macOS / Linux).
#
# This installs the `clashforgectl` remote-control CLI onto THIS machine
# (the one you SSH/manage routers *from*), as a system-level command on your
# PATH. It is not the router-side installer — see clashforgectl.sh for that
# (SSH into the router and run it there instead).
#
# Install only:
#   curl -fsSL https://dl.wei1xuan.com/install.sh | sh
#
# Install (or upgrade to latest) and immediately run a command — `sh -s --`
# forwards everything after `--` as positional args to the piped script:
#   curl -fsSL https://dl.wei1xuan.com/install.sh | sh -s -- --router 192.168.20.1 upgrade
#
# After installing, open a new terminal and `clashforgectl` works from any
# directory.

set -e

BASE_URL="https://dl.wei1xuan.com/releases/latest"
INSTALL_DIR="$HOME/.local/bin"
BIN_PATH="$INSTALL_DIR/clashforgectl"

log()  { printf '\033[36m[install]\033[0m %s\n' "$1"; }
die()  { printf '\033[31m[install] ERROR: %s\033[0m\n' "$1" >&2; exit 1; }

mkdir -p "$INSTALL_DIR"

log "Installing clashforgectl to $BIN_PATH ..."
curl -fsSL "$BASE_URL/clashforgectl" -o "$BIN_PATH" || die "download failed"
chmod +x "$BIN_PATH"

# Add ~/.local/bin to PATH for future shells if it isn't already there.
# Many distros already add it via /etc/profile.d — only touch rc files if needed.
case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        log "$INSTALL_DIR is already on your PATH."
        ;;
    *)
        line='export PATH="$HOME/.local/bin:$PATH"'
        updated=""
        for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
            if [ -f "$rc" ] && ! grep -qF '.local/bin' "$rc" 2>/dev/null; then
                printf '\n# added by clashforgectl installer\n%s\n' "$line" >> "$rc"
                log "Added \$HOME/.local/bin to PATH in $rc"
                updated=1
            fi
        done
        if [ -z "$updated" ] && [ ! -f "$HOME/.bashrc" ] && [ ! -f "$HOME/.zshrc" ] && [ ! -f "$HOME/.profile" ]; then
            printf '%s\n' "$line" >> "$HOME/.profile"
            log "Created \$HOME/.profile with \$HOME/.local/bin on PATH"
        fi
        export PATH="$INSTALL_DIR:$PATH"
        ;;
esac

log "clashforgectl installed ✓ — open a new terminal and run 'clashforgectl' from anywhere."

if [ "$#" -gt 0 ]; then
    log "Running: clashforgectl $*"
    exec "$BIN_PATH" "$@"
fi
