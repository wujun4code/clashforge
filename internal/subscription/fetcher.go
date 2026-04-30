package subscription

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

const defaultUserAgent = "clash-meta"

// Fetch downloads subscription content from url.
func Fetch(url, userAgent string) ([]byte, error) {
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	var lastErr error
	client := &http.Client{Timeout: 15 * time.Second}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		data, err := doFetch(client, url, userAgent)
		if err == nil {
			log.Info().
				Str("url", url).
				Int("size", len(data)).
				Int("attempt", attempt+1).
				Msg("subscription: 订阅下载成功")
			return data, nil
		}
		lastErr = err
		log.Warn().
			Str("url", url).
			Int("attempt", attempt+1).
			Err(err).
			Msg("subscription: 订阅下载失败，将重试")
	}
	log.Error().
		Str("url", url).
		Err(lastErr).
		Msg("subscription: ⚠️ 订阅下载全部失败！无法获取代理节点，国际流量将无法代理")
	return nil, fmt.Errorf("fetch subscription: %w", lastErr)
}

func doFetch(client *http.Client, url, userAgent string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
