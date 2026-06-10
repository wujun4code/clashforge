// Package mihomobridge embeds the mihomo core inside the iOS PacketTunnel
// network extension.  gomobile bind compiles this package into
// Mihomobridge.xcframework; the Swift PacketTunnelProvider drives it.
//
// iOS forbids fork/exec, so unlike Android (which spawns libmihomo.so as a
// child process and smuggles the TUN fd through stdin) the core runs
// in-process.  The extension finds the utun fd Apple created for the tunnel
// and PatchConfigWithTun writes it into config.yaml as tun.file-descriptor —
// sing-tun's darwin backend adopts any fd > 0 directly, no patching needed.
//
// The packet-tunnel process has a hard ~50 MB jetsam limit, hence the
// aggressive GC tuning in Start (Android uses GOGC=40 / GOMEMLIMIT=100MiB
// with far more headroom).
package mihomobridge

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	mihomoconfig "github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
	mihomolog "github.com/metacubex/mihomo/log"
)

// LogCallback receives mihomo core log lines in Swift.
type LogCallback interface {
	OnLog(level string, payload string)
}

var (
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
)

// Start parses config.yaml under homeDir and brings up the core
// (TUN listener, DNS, external-controller on 127.0.0.1:9090).
// Blocking work happens inside; returns once the config is applied.
func Start(homeDir string, configPath string, callback LogCallback) error {
	mu.Lock()
	defer mu.Unlock()
	if running {
		return nil
	}

	// ~50 MB jetsam ceiling for the whole extension process; leave room for
	// Swift/runtime overhead.  SetMemoryLimit makes the Go runtime scavenge
	// aggressively as the soft limit nears instead of OOM-ing the process.
	debug.SetGCPercent(30)
	debug.SetMemoryLimit(40 << 20)

	C.SetHomeDir(homeDir)
	C.SetConfig(configPath)
	if err := mihomoconfig.Init(C.Path.HomeDir()); err != nil {
		return fmt.Errorf("config init: %w", err)
	}

	stop := make(chan struct{})

	if callback != nil {
		sub := mihomolog.Subscribe()
		go func() {
			defer mihomolog.UnSubscribe(sub)
			for {
				select {
				case ev, ok := <-sub:
					if !ok {
						return
					}
					callback.OnLog(ev.LogLevel.String(), ev.Payload)
				case <-stop:
					return
				}
			}
		}()
	}

	if err := hub.Parse(nil); err != nil {
		close(stop)
		return fmt.Errorf("hub parse: %w", err)
	}

	// Return freed pages to the OS periodically; jetsam accounts dirty pages,
	// not Go heap, so prompt scavenging is what keeps the extension alive.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				debug.FreeOSMemory()
			case <-stop:
				return
			}
		}
	}()

	stopCh = stop
	running = true
	return nil
}

// Stop shuts the core down.  The extension process usually exits right
// after, but a clean shutdown closes the TUN fd and listener sockets so a
// quick reconnect doesn't race the old instance.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if !running {
		return
	}
	executor.Shutdown()
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
	running = false
}

// IsRunning reports whether Start has completed successfully.
func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return running
}

// ForceGC is called by Swift on didReceiveMemoryWarning.
func ForceGC() {
	debug.FreeOSMemory()
}
