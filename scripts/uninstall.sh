#!/bin/sh
# ClashForge remote-management CLI uninstaller (macOS / Linux).
#
# Removes what install.sh installed: the clashforgectl binary from
# ~/.local/bin and the PATH entry it added to your shell rc file(s). Only
# removes the binary itself, never the whole ~/.local/bin directory — that's
# shared by other tools. Does not touch anything on a router.
#
# Usage:
#   curl -fsSL https://dl.wei1xuan.com/uninstall.sh | sh

set -e

INSTALL_DIR="$HOME/.local/bin"
BIN_PATH="$INSTALL_DIR/clashforgectl"
MARKER="# added by clashforgectl installer"

log() { printf '\033[36m[uninstall]\033[0m %s\n' "$1"; }

# ── 1. Remove the binary ──────────────────────────────────────────────────────
if [ -f "$BIN_PATH" ]; then
    rm -f "$BIN_PATH"
    log "Removed $BIN_PATH"
else
    log "$BIN_PATH does not exist (already uninstalled)"
fi

# ── 2. Remove the PATH entry we added, identified by our own marker comment ───
# install.sh only ever appends "$MARKER\nexport PATH=...\n" as a pair — strip
# exactly that pair, nothing else in the file.
removed_any=""
for rc in "$HOME/.bashrc" "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$rc" ] && grep -qF "$MARKER" "$rc" 2>/dev/null; then
        tmp=$(mktemp)
        awk -v marker="$MARKER" '
            index($0, marker) == 1 { skip=1; next }
            skip > 0 { skip--; next }
            { print }
        ' "$rc" > "$tmp"
        mv "$tmp" "$rc"
        log "Removed PATH entry from $rc"
        removed_any=1
    fi
done
[ -n "$removed_any" ] || log "No installer-added PATH entries found"

log "Uninstall complete."
