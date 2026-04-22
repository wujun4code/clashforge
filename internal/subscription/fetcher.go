package subscription

import (
	"fmt"
	"io"
	"net/http"
	"time"
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
			return data, nil
		}
		lastErr = err
	}
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
