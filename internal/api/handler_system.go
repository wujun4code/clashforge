package api

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// handleStopService executes a named stop operation on the router.
// Accepted targets: "openclash", "clashforge-full"
// "clashforge-full" stops ClashForge itself (init.d + nft cleanup + DNS restore).
// This handler runs in a goroutine with a generous timeout so the shell script
// can complete even if the underlying service kills our process.
func handleStopService(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Target string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			Err(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
			return
		}

		var script string
		switch body.Target {
		case "openclash":
			script = stopOpenClashScript()
		case "clashforge-full":
			script = stopClashForgeScript()
		default:
			Err(w, http.StatusBadRequest, "UNKNOWN_TARGET", "target must be 'openclash' or 'clashforge-full'")
			return
		}

		// Run the script with a 30-second timeout.
		// Use sh -c so we don't need a file on disk.
		cmd := exec.Command("sh", "-c", script)
		out, err := withTimeout(cmd, 30*time.Second)
		if err != nil {
			Err(w, http.StatusInternalServerError, "SCRIPT_FAILED", strings.TrimSpace(string(out))+" | "+err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"target": body.Target,
			"output": strings.TrimSpace(string(out)),
		})
	}
}

// withTimeout runs cmd and captures combined output, killing it after d.
func withTimeout(cmd *exec.Cmd, d time.Duration) ([]byte, error) {
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		defer close(done)
		out, runErr = cmd.CombinedOutput()
	}()
	select {
	case <-done:
		return out, runErr
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return out, nil
	}
}

// stopOpenClashScript returns an inline shell script that fully stops OpenClash.
func stopOpenClashScript() string {
	return `
set -e
log() { echo "[$(date '+%H:%M:%S')] $1"; }

log "Stopping openclash init.d..."
/etc/init.d/openclash stop 2>/dev/null || true

log "Killing watchdog..."
WPIDS=$(pgrep -f openclash_watchdog 2>/dev/null || true)
[ -n "$WPIDS" ] && kill $WPIDS 2>/dev/null || true
sleep 1
WPIDS=$(pgrep -f openclash_watchdog 2>/dev/null || true)
[ -n "$WPIDS" ] && kill -9 $WPIDS 2>/dev/null || true

log "Killing clash kernel..."
CPIDS=$(pgrep -f "/etc/openclash/clash" 2>/dev/null || true)
[ -n "$CPIDS" ] && kill $CPIDS 2>/dev/null || true
sleep 1
CPIDS=$(pgrep -f "/etc/openclash/clash" 2>/dev/null || true)
[ -n "$CPIDS" ] && kill -9 $CPIDS 2>/dev/null || true

log "Cleaning nftables openclash chains..."
for CHAIN in openclash openclash_mangle openclash_mangle_output openclash_output openclash_upnp openclash_wan_input; do
  HANDLES=$(nft -a list table inet fw4 2>/dev/null | grep "jump ${CHAIN}" | grep -oE 'handle [0-9]+' | awk '{print $2}' || true)
  for H in $HANDLES; do
    PARENT=$(nft -a list table inet fw4 2>/dev/null | awk "/chain /{cur=\$2} /jump ${CHAIN}.*handle ${H}/{print cur}" || true)
    [ -n "$PARENT" ] && nft delete rule inet fw4 "$PARENT" handle "$H" 2>/dev/null || true
  done
  if nft list chain inet fw4 "$CHAIN" >/dev/null 2>&1; then
    nft flush chain inet fw4 "$CHAIN" 2>/dev/null || true
    nft delete chain inet fw4 "$CHAIN" 2>/dev/null || true
  fi
done

HANDLES=$(nft -a list table inet fw4 2>/dev/null | grep 'comment "OpenClash' | grep -oE 'handle [0-9]+' | awk '{print $2}' || true)
for H in $HANDLES; do
  PARENT=$(nft -a list table inet fw4 2>/dev/null | awk "/chain /{cur=\$2} /comment \"OpenClash.*handle ${H}/{print cur}" || true)
  [ -n "$PARENT" ] && nft delete rule inet fw4 "$PARENT" handle "$H" 2>/dev/null || true
done

log "Cleaning fake-ip route..."
ip route del 198.18.0.0/16 2>/dev/null || true

log "Reloading dnsmasq..."
/etc/init.d/dnsmasq reload 2>/dev/null || /etc/init.d/dnsmasq restart 2>/dev/null || true

log "Done."
`
}

// stopClashForgeScript returns an inline shell script that fully stops ClashForge.
// NOTE: This will also kill the current clashforge process that is executing this
// script's parent request. The HTTP response is sent before the self-kill step.
func stopClashForgeScript() string {
	return `
set -e
log() { echo "[$(date '+%H:%M:%S')] $1"; }

log "Stopping clashforge init.d..."
/etc/init.d/clashforge stop 2>/dev/null || true
sleep 1

log "Killing mihomo-clashforge kernel..."
MPIDS=$(pgrep -f "/usr/bin/mihomo-clashforge" 2>/dev/null || true)
[ -n "$MPIDS" ] && kill $MPIDS 2>/dev/null || true
sleep 1
MPIDS=$(pgrep -f "/usr/bin/mihomo-clashforge" 2>/dev/null || true)
[ -n "$MPIDS" ] && kill -9 $MPIDS 2>/dev/null || true

log "Deleting nftables table inet metaclash..."
nft delete table inet metaclash 2>/dev/null || true

log "Cleaning policy routing (fwmark 0x1a3)..."
while ip rule show 2>/dev/null | grep -q 'fwmark 0x1a3'; do
  ip rule del fwmark 0x1a3 lookup 100 2>/dev/null || break
done
ip route flush table 100 2>/dev/null || true

log "Restoring dnsmasq..."
rm -f /etc/dnsmasq.d/clashforge.conf 2>/dev/null || true
/etc/init.d/dnsmasq restart 2>/dev/null || true

log "Cleaning pid file..."
rm -f /var/run/metaclash/metaclash.pid 2>/dev/null || true

log "Killing clashforge main process..."
CFPIDS=$(pgrep -f "/usr/bin/clashforge" 2>/dev/null || true)
[ -n "$CFPIDS" ] && kill $CFPIDS 2>/dev/null || true

log "Done."
`
}
