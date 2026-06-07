package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var endpoints = []struct {
	url  string
	host string
}{
	{url: "http://1.1.1.1/cdn-cgi/trace", host: "www.cloudflare.com"},
	{url: "http://api.ipify.org?format=json"},
	{url: "http://api4.ipify.org?format=json"},
	{url: "http://httpbin.org/ip"},
}

func main() {
	ip, err := probeIP()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(ip)
}

func probeIP() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	var errs []string
	for _, endpoint := range endpoints {
		req, err := http.NewRequest(http.MethodGet, endpoint.url, nil)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if endpoint.host != "" {
			req.Host = endpoint.host
		}
		req.Header.Set("User-Agent", "clashforge-android-e2e-tun-probe/1.0")
		resp, err := client.Do(req)
		if err != nil {
			errs = append(errs, endpoint.url+": "+err.Error())
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if readErr != nil {
			errs = append(errs, endpoint.url+": "+readErr.Error())
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			errs = append(errs, fmt.Sprintf("%s: HTTP %d", endpoint.url, resp.StatusCode))
			continue
		}
		ip := extractIP(body)
		if ip != "" {
			return ip, nil
		}
		errs = append(errs, endpoint.url+": no IP in response")
	}
	return "", errors.New(strings.Join(errs, "; "))
}

func extractIP(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "ip="); ok {
			if value != "" {
				return value
			}
		}
	}

	var data map[string]any
	if json.Unmarshal(body, &data) == nil {
		for _, key := range []string{"ip", "origin"} {
			if value, ok := data[key].(string); ok {
				ip := strings.TrimSpace(strings.Split(value, ",")[0])
				if ip != "" {
					return ip
				}
			}
		}
	}
	return strings.TrimSpace(string(body))
}
