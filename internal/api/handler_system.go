package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wujun4code/clashforge/internal/config"
)

// ConflictService describes a service that may conflict with ClashForge.
type ConflictService struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Running bool   `json:"running"`
	PIDs    []int  `json:"pids,omitempty"`
}

// handleDetectConflicts checks for known services that conflict with ClashForge.
func handleDetectConflicts(_ Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		services := []ConflictService{
			detectConflict("openclash", "OpenClash", []string{"/etc/openclash/clash", "openclash_watchdog"}),
			detectConflict("mihomo", "系统 mihomo（非 ClashForge 管理）", []string{"/usr/bin/mihomo"}),
			detectConflict("clash", "Clash（原版）", []string{"/usr/bin/clash"}),
		}

		// Only return services that are actually running
		running := make([]ConflictService, 0)
		for _, s := range services {
			if s.Running {
				running = append(running, s)
			}
		}

		JSON(w, http.StatusOK, map[string]any{
			"conflicts":    running,
			"has_conflict": len(running) > 0,
		})
	}
}

// detectConflict checks /proc for any process matching the given cmdline patterns.
func detectConflict(name, label string, patterns []string) ConflictService {
	svc := ConflictService{Name: name, Label: label}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return svc
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := 0
		for _, c := range e.Name() {
			if c < '0' || c > '9' {
				pid = -1
				break
			}
			pid = pid*10 + int(c-'0')
		}
		if pid <= 0 {
			continue
		}
		cmdlineBytes, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		cmdline := strings.ReplaceAll(string(cmdlineBytes), "\x00", " ")
		for _, pat := range patterns {
			if strings.Contains(cmdline, pat) {
				// Skip our own processes
				if strings.Contains(cmdline, "mihomo-clashforge") ||
					strings.Contains(cmdline, "/usr/bin/clashforge") {
					break
				}
				svc.Running = true
				svc.PIDs = append(svc.PIDs, pid)
				break
			}
		}
	}
	return svc
}

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
		out, _ := withTimeout(cmd, 30*time.Second)

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
log() { echo "[$(date '+%H:%M:%S')] $1"; }

log "Killing watchdog + clash kernel (pass 1)..."
for pid in $(ls /proc | grep -E '^[0-9]+$'); do
  cmdfile="/proc/$pid/cmdline"
  [ -f "$cmdfile" ] || continue
  cmd=$(tr '\0' '\n' < "$cmdfile" 2>/dev/null | head -1)
  case "$cmd" in
    *openclash*|*/etc/openclash/clash) kill -9 "$pid" 2>/dev/null ;;
  esac
done; true
sleep 1

log "Killing clash kernel (pass 2, paranoid)..."
for pid in $(ls /proc | grep -E '^[0-9]+$'); do
  cmdfile="/proc/$pid/cmdline"
  [ -f "$cmdfile" ] || continue
  cmd=$(tr '\0' '\n' < "$cmdfile" 2>/dev/null | head -1)
  case "$cmd" in
    *openclash*|*/etc/openclash/clash) kill -9 "$pid" 2>/dev/null ;;
  esac
done; true

log "Stopping openclash init.d..."
/etc/init.d/openclash stop 2>/dev/null; true

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
uci -q delete dhcp.@dnsmasq[0].port 2>/dev/null || true
uci -q commit dhcp 2>/dev/null || true
/etc/init.d/dnsmasq restart 2>/dev/null || true

log "Cleaning pid file..."
rm -f /var/run/metaclash/metaclash.pid 2>/dev/null || true

log "Killing clashforge main process..."
CFPIDS=$(pgrep -f "/usr/bin/clashforge" 2>/dev/null || true)
[ -n "$CFPIDS" ] && kill $CFPIDS 2>/dev/null || true

log "Done."
`
}

// handleResetClashForge resets ClashForge to a clean factory state:
//  1. Stop mihomo core + release nft/DNS takeovers
//  2. Wipe user data (subscriptions, overrides, sources, generated config, runtime files)
//  3. Rewrite config.toml to defaults (preserving api_secret and ui port)
//  4. Re-exec this process so it boots fresh with an empty state
func handleResetClashForge(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Stop core gracefully (best-effort)
		_ = deps.Core.Stop()

		// 2. Release nft / DNS takeovers (best-effort inline shell)
		if deps.Netfilter != nil {
			_ = deps.Netfilter.Cleanup()
		}
		_ = exec.Command("sh", "-c",
			"nft delete table inet metaclash 2>/dev/null; "+
				"while ip rule show 2>/dev/null | grep -q 'fwmark 0x1a3'; do "+
				"  ip rule del fwmark 0x1a3 lookup 100 2>/dev/null || break; done; "+
				"ip route flush table 100 2>/dev/null; "+
				"rm -f /etc/dnsmasq.d/clashforge.conf; "+
				"uci -q delete dhcp.@dnsmasq[0].port 2>/dev/null || true; "+
				"uci -q commit dhcp 2>/dev/null || true; "+
				"/etc/init.d/dnsmasq restart 2>/dev/null || true").Run()

		dataDir := deps.Config.Core.DataDir
		runtimeDir := deps.Config.Core.RuntimeDir

		// 3. Delete user-generated data — keep directory itself so process can restart
		for _, p := range []string{
			filepath.Join(dataDir, "overrides.yaml"),
			filepath.Join(dataDir, "subscriptions.json"),
			filepath.Join(dataDir, "sources"),
			filepath.Join(dataDir, "active_source.json"),
			filepath.Join(runtimeDir, "mihomo-config.yaml"),
			filepath.Join(runtimeDir, "mihomo.pid"),
			filepath.Join(runtimeDir, "metaclash.pid"),
		} {
			_ = os.RemoveAll(p)
		}

		// 4. Reset config.toml to defaults, preserving api_secret + ui port
		def := config.Default()
		def.Security.APISecret = deps.Config.Security.APISecret
		def.Ports.UI = deps.Config.Ports.UI
		if err := config.Save(deps.ConfigPath, def); err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_WRITE_FAILED", err.Error())
			return
		}

		// 5. Flush response before re-exec so client receives the reply
		JSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"message": "ClashForge 已重置为出厂状态，进程即将重启",
		})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		time.Sleep(300 * time.Millisecond)

		// 6. Re-exec: replace this process image with a fresh start
		exe, err := os.Executable()
		if err != nil {
			return
		}
		_ = syscall.Exec(exe, os.Args, os.Environ())
	}
}
