package quickstart

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
)

func TestQuickstartHealthFailureCanRetry(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	var healthy atomic.Bool
	quickstartHealthCore(t, &deps, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	if err := autoConfigureClashForge(ctx, deps, func(Event) {}, "", "", "", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded unhealthy error: %v", err)
	}
	oldPID := deps.Core.Status().PID
	healthy.Store(true)
	if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err != nil {
		t.Fatalf("same quickstart dependencies could not retry: %v", err)
	}
	if deps.Core.Status().PID == oldPID || !deps.Netfilter.IsApplied() {
		t.Fatal("retry did not restart and acquire takeover")
	}
}

func TestQuickstartOwnerFailuresRemainRetryable(t *testing.T) {
	for _, failure := range []string{"uci:commit dhcp", "ip:-4 rule del"} {
		t.Run(failure, func(t *testing.T) {
			deps, record := quickstartLifecycleDeps(t)
			if err := dns.Setup(dns.ModeUpstream, deps.Config.Ports.DNS); err != nil {
				t.Fatal(err)
			}
			old := netfilter.NewManager(netfilter.Config{Mode: "tun", FirewallBackend: "nftables", TunDevice: "Meta"})
			if err := old.Apply(); err != nil {
				t.Fatal(err)
			}
			deps.Netfilter = old
			if err := os.WriteFile(record, nil, 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CF_FAIL_COMMAND", failure)
			if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err == nil {
				t.Fatal("old owner failure was suppressed")
			}
			if deps.Netfilter != old || deps.Core.Status().PID != 0 {
				t.Fatal("old owner discarded or core started before cleanup succeeded")
			}
			t.Setenv("CF_FAIL_COMMAND", "")
			if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err != nil {
				t.Fatalf("retry through same owner failed: %v", err)
			}
			if !deps.Core.Status().Ready || !deps.Netfilter.IsApplied() {
				t.Fatal("retry failed to acquire new takeover")
			}
		})
	}
}

func TestQuickstartHealthInFlightCancellation(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	var block atomic.Bool
	entered := make(chan struct{}, 1)
	quickstartHealthCore(t, &deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if block.Load() {
			entered <- struct{}{}
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	if err := deps.Core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		select {
		case <-entered:
			cancel()
		case <-ctx.Done():
		}
	}()
	start := time.Now()
	err := waitQuickstartCoreHealthy(ctx, deps.Core)
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatalf("in-flight HTTP cancellation was not bounded/preserved: %v elapsed=%s", err, time.Since(start))
	}
}

func TestQuickstartHealthRequestTimeoutCanRecover(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	var attempts atomic.Int32
	var probe atomic.Bool
	quickstartHealthCore(t, &deps, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if probe.Load() && attempts.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	if err := deps.Core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	probe.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := waitQuickstartCoreHealthy(ctx, deps.Core); err != nil {
		t.Fatalf("did not recover after per-request timeout: %v", err)
	}
	if attempts.Load() < 2 || time.Since(start) > 2*time.Second {
		t.Fatalf("1s per-request timeout not observed: attempts=%d elapsed=%s", attempts.Load(), time.Since(start))
	}
}

func TestQuickstartHealthConnectionRefused(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	server := quickstartHealthCore(t, &deps, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err := deps.Core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := waitQuickstartCoreHealthy(ctx, deps.Core)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "connection refused") || time.Since(start) > time.Second {
		t.Fatalf("real connection refusal was not rejected/bounded: err=%v elapsed=%s", err, time.Since(start))
	}
	t.Logf("manager_ready=%t connection_error=%v elapsed=%s", deps.Core.Status().Ready, err, time.Since(start))
}

func TestQuickstartHealthDefaultDeadline(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	quickstartHealthCore(t, &deps, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	if err := deps.Core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err := waitQuickstartCoreHealthy(context.Background(), deps.Core)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) || elapsed < 4500*time.Millisecond || elapsed > 6*time.Second {
		t.Fatalf("default five-second deadline: err=%v elapsed=%s", err, elapsed)
	}
	t.Logf("manager_ready=%t control_health_error=%v elapsed=%s", deps.Core.Status().Ready, err, elapsed)
}
