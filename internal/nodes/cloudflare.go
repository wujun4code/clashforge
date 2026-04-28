package nodes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const cloudflareAPI = "https://api.cloudflare.com/client/v4"

type CloudflareZone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type cloudflareEnvelope[T any] struct {
	Success bool `json:"success"`
	Result  T    `json:"result"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func cloudflareRequest(method, path, token string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, cloudflareAPI+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cloudflare api status %d: %s", resp.StatusCode, string(raw))
	}
	return raw, nil
}

func CloudflareListZones(token, accountID string) ([]CloudflareZone, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("CF API Token 不能为空")
	}
	path := "/zones?per_page=100"
	if strings.TrimSpace(accountID) != "" {
		path += "&account.id=" + accountID
	}
	raw, err := cloudflareRequest(http.MethodGet, path, token, nil)
	if err != nil {
		return nil, err
	}
	var env cloudflareEnvelope[[]CloudflareZone]
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.Success {
		if len(env.Errors) > 0 {
			return nil, fmt.Errorf(env.Errors[0].Message)
		}
		return nil, fmt.Errorf("cloudflare api 返回失败")
	}
	return env.Result, nil
}

func findZoneIDByDomain(token, accountID, domain string) (string, error) {
	zones, err := CloudflareListZones(token, accountID)
	if err != nil {
		return "", err
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	best := ""
	for _, z := range zones {
		zn := strings.ToLower(strings.TrimSpace(z.Name))
		if domain == zn || strings.HasSuffix(domain, "."+zn) {
			if len(zn) > len(best) {
				best = z.ID
			}
		}
	}
	if best == "" {
		return "", fmt.Errorf("未在 Cloudflare 账户中找到匹配域名: %s", domain)
	}
	return best, nil
}

func CloudflareUpsertARecord(token, zoneID, fqdn, ip string) error {
	type dnsRecord struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if strings.TrimSpace(zoneID) == "" {
		return fmt.Errorf("Zone ID 不能为空")
	}
	listRaw, err := cloudflareRequest(http.MethodGet, fmt.Sprintf("/zones/%s/dns_records?type=A&name=%s", zoneID, fqdn), token, nil)
	if err != nil {
		return err
	}
	var listEnv cloudflareEnvelope[[]dnsRecord]
	if err := json.Unmarshal(listRaw, &listEnv); err != nil {
		return err
	}
	if !listEnv.Success {
		return fmt.Errorf("cloudflare 查询 DNS 记录失败")
	}

	payload := map[string]any{
		"type":    "A",
		"name":    fqdn,
		"content": ip,
		"ttl":     120,
		"proxied": false,
	}

	if len(listEnv.Result) == 0 {
		_, err = cloudflareRequest(http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), token, payload)
		return err
	}

	recordID := listEnv.Result[0].ID
	_, err = cloudflareRequest(http.MethodPut, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID), token, payload)
	return err
}
