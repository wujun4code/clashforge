package quickstart

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wujun4code/clashforge/internal/core"
)

// Use the real CoreManager with a harmless owned child and real loopback HTTP.
// NF commands and typed UCI are isolated by quickstartLifecycleDeps' PATH.
func quickstartHealthCore(t *testing.T, deps *Deps, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	apiPort, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	cm := core.NewManager(core.CoreManagerConfig{
		Binary:     deps.Core.Status().Binary,
		ConfigFile: filepath.Join(deps.Config.Core.RuntimeDir, "mihomo-config.yaml"),
		APIPort:    apiPort,
	})
	deps.Core = cm
	t.Cleanup(func() { _ = cm.Stop() })
	return server
}

func TestQuickstartControllerHealthGate(t *testing.T) {
	for _, scenario := range []string{"http503", "redirect", "connection-refused", "cancelled"} {
		for _, alreadyRunning := range []bool{false, true} {
			branch := "Start"
			if alreadyRunning {
				branch = "Restart"
			}
			t.Run(scenario+"/"+branch, func(t *testing.T) {
				deps, record := quickstartLifecycleDeps(t)
				beforeDNS := startupDNSState(t)
				var response atomic.Int32
				response.Store(http.StatusOK)
				server := quickstartHealthCore(t, &deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/healthy" {
						w.WriteHeader(http.StatusOK)
						return
					}
					if response.Load() == http.StatusFound {
						w.Header().Set("Location", "/healthy")
					}
					w.WriteHeader(int(response.Load()))
				}))
				if alreadyRunning {
					if err := deps.Core.Start(context.Background()); err != nil {
						t.Fatal(err)
					}
				}
				oldPID := deps.Core.Status().PID
				switch scenario {
				case "http503":
					response.Store(http.StatusServiceUnavailable)
				case "redirect":
					response.Store(http.StatusFound)
				case "connection-refused":
					server.Close()
				}
				ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
				defer cancel()
				if scenario == "cancelled" {
					cancel()
				}
				var events []Event
				start := time.Now()
				err := autoConfigureClashForge(ctx, deps, func(e Event) { events = append(events, e) }, "", "", "", nil)
				elapsed := time.Since(start)
				status := deps.Core.Status()
				data, readErr := os.ReadFile(record)
				if readErr != nil && !os.IsNotExist(readErr) {
					t.Fatal(readErr)
				}
				trace := string(data)
				t.Logf("scenario=%s branch=%s old_pid=%d new_pid=%d manager_ready=%t elapsed=%s error=%v nf_applied=%t dns_write=%t", scenario, branch, oldPID, status.PID, status.Ready, elapsed, err, deps.Netfilter.IsApplied(), strings.Contains(trace, "server=127.0.0.1#"))
				for _, e := range events {
					t.Logf("event=%+v", e)
				}
				if status.PID <= 0 || (alreadyRunning && status.PID == oldPID) {
					t.Fatal("did not exercise actual Start/Restart")
				}
				if err == nil {
					t.Error("unhealthy controller reported quickstart success")
				} else if !strings.Contains(err.Error(), "quickstart core API") {
					t.Errorf("failed before intended health gate: %v", err)
				}
				if scenario == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Errorf("cancellation not preserved: %v", err)
				}
				if elapsed > 3*time.Second {
					t.Errorf("negative check was not bounded by short parent context: %s", elapsed)
				}
				if deps.Netfilter.IsApplied() || strings.Contains(trace, "nft -f -") || strings.Contains(trace, "server=127.0.0.1#") {
					t.Error("unhealthy controller acquired new NF/DNS resources")
				}
				assertStartupDNSEqual(t, beforeDNS)
				for _, e := range events {
					if e.Status == StatusOK && (e.Step == "start_core" || e.Step == "apply_netfilter" || e.Step == "apply_dns") {
						t.Errorf("false success event: %+v", e)
					}
				}
			})
		}
	}
}

func TestQuickstartHealthyControllerAcquiresTakeover(t *testing.T) {
	for _, alreadyRunning := range []bool{false, true} {
		t.Run(strconv.FormatBool(alreadyRunning), func(t *testing.T) {
			deps, record := quickstartLifecycleDeps(t)
			if alreadyRunning {
				if err := deps.Core.Start(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			oldPID := deps.Core.Status().PID
			var events []Event
			if err := autoConfigureClashForge(context.Background(), deps, func(e Event) { events = append(events, e) }, "", "", "", nil); err != nil {
				t.Fatal(err)
			}
			status := deps.Core.Status()
			if !status.Ready || status.PID <= 0 || (alreadyRunning && status.PID == oldPID) {
				t.Fatalf("healthy Start/Restart failed: old PID %d status %+v", oldPID, status)
			}
			data, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			if !deps.Netfilter.IsApplied() || !strings.Contains(string(data), "nft -f -") || !strings.Contains(string(data), "server=127.0.0.1#") {
				t.Fatalf("healthy controller did not acquire NF/DNS: %s", data)
			}
			want := []string{"start_core", "apply_netfilter", "apply_dns"}
			var got []string
			for _, e := range events {
				if e.Status == StatusOK && (e.Step == "start_core" || e.Step == "apply_netfilter" || e.Step == "apply_dns") {
					got = append(got, e.Step)
				}
			}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("success event ordering: got %v want %v", got, want)
			}
			t.Logf("HTTP200 already_running=%t old_pid=%d new_pid=%d manager_ready=%t nf_applied=%t DNS_acquired=true", alreadyRunning, oldPID, status.PID, status.Ready, deps.Netfilter.IsApplied())
		})
	}
}
