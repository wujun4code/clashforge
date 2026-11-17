package quickstart

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/wujun4code/clashforge/internal/core"
)

// waitQuickstartCoreHealthy checks only controller readiness, not DNS or the
// data plane. CoreManager may report Ready after its non-fatal API wait fails.
func waitQuickstartCoreHealthy(ctx context.Context, cm *core.CoreManager) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout:       time.Second,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("quickstart core API is not healthy (last check: %v): %w", lastErr, err)
		}
		status := cm.Status()
		if !status.Ready {
			return fmt.Errorf("quickstart core API is not healthy: core is not running")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/version", status.APIPort), nil)
		if err != nil {
			return fmt.Errorf("quickstart core API request: %w", err)
		}
		resp, err := client.Do(req)
		lastErr = err
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && ctx.Err() == nil {
				return nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("quickstart core API is not healthy (last check: %v): %w", lastErr, ctx.Err())
		case <-ticker.C:
		}
	}
}
