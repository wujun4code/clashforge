package azure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	managementEndpoint = "https://management.azure.com"
	authEndpoint       = "https://login.microsoftonline.com"
	computeAPIVersion  = "2024-07-01"
	networkAPIVersion  = "2024-05-01"
	resourceAPIVersion = "2021-04-01"
)

// Config holds Azure Service Principal credentials.
type Config struct {
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	SubscriptionID string `json:"subscription_id"`
}

// tokenResponse is the OAuth2 token response from Azure AD.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   string `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// GetAccessToken fetches a Bearer token for the Azure management plane
// using the client credentials grant (Service Principal).
func GetAccessToken(cfg Config) (string, error) {
	if strings.TrimSpace(cfg.TenantID) == "" {
		return "", fmt.Errorf("tenant_id 不能为空")
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return "", fmt.Errorf("client_id 不能为空")
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return "", fmt.Errorf("client_secret 不能为空")
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", cfg.ClientID)
	data.Set("client_secret", cfg.ClientSecret)
	data.Set("scope", "https://management.azure.com/.default")

	tokenURL := fmt.Sprintf("%s/%s/oauth2/v2.0/token", authEndpoint, cfg.TenantID)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("获取 Azure 令牌失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取令牌响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Try to parse the error description
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if je := json.Unmarshal(raw, &errResp); je == nil && errResp.Error != "" {
			return "", fmt.Errorf("Azure 认证失败 (%s): %s", errResp.Error, errResp.ErrorDescription)
		}
		return "", fmt.Errorf("Azure 认证失败 (HTTP %d): %s", resp.StatusCode, string(raw))
	}

	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return "", fmt.Errorf("解析令牌响应失败: %w", err)
	}
	return tok.AccessToken, nil
}

// armRequest performs an ARM REST API call and returns the response body.
func armRequest(method, path, token string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, managementEndpoint+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("ARM API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读取响应失败: %w", err)
	}

	return raw, resp.StatusCode, nil
}

// armRequestWithRetry polls an ARM async operation until it completes.
// Azure returns 202 Accepted with an Azure-AsyncOperation header for LRO.
func armPollAsyncOp(asyncURL, token string, timeoutSec int) error {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		raw, status, err := armRequest("GET", strings.TrimPrefix(asyncURL, managementEndpoint), token, nil)
		if err != nil {
			// If it's an absolute URL outside our management endpoint, use direct fetch
			if strings.HasPrefix(asyncURL, "https://") {
				raw, status, err = armDirectGet(asyncURL, token)
			}
			if err != nil {
				return err
			}
		}
		if status == http.StatusNotFound {
			return fmt.Errorf("async 操作资源未找到")
		}

		var opStatus struct {
			Status string `json:"status"`
			Error  *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &opStatus); err != nil {
			return fmt.Errorf("解析异步操作状态失败: %w", err)
		}

		switch strings.ToLower(opStatus.Status) {
		case "succeeded":
			return nil
		case "failed", "canceled":
			if opStatus.Error != nil {
				return fmt.Errorf("操作失败 (%s): %s", opStatus.Error.Code, opStatus.Error.Message)
			}
			return fmt.Errorf("操作以状态 %q 结束", opStatus.Status)
		}
		// InProgress / Running - keep polling
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("等待操作完成超时")
}

func armDirectGet(fullURL, token string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, err
}

func armGetJSON(path, token string, out any) error {
	raw, status, err := armRequest("GET", path, token, nil)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return armParseError(raw, status)
	}
	return json.Unmarshal(raw, out)
}

func armPutJSON(path, token string, body any, out any) (string, error) {
	raw, status, err := armRequest("PUT", path, token, body)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", armParseError(raw, status)
	}
	if out != nil {
		_ = json.Unmarshal(raw, out)
	}
	return "", nil
}

func armPutJSONAsync(path, token string, body any) (string, error) {
	var reader io.Reader
	raw2, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	reader = bytes.NewReader(raw2)

	req, err := http.NewRequest("PUT", managementEndpoint+path, reader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ARM 请求失败: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", armParseError(raw, resp.StatusCode)
	}

	// Return the async operation URL if present
	asyncURL := resp.Header.Get("Azure-AsyncOperation")
	if asyncURL == "" {
		asyncURL = resp.Header.Get("Location")
	}
	return asyncURL, nil
}

func armParseError(raw []byte, status int) error {
	var errEnvelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if je := json.Unmarshal(raw, &errEnvelope); je == nil && errEnvelope.Error.Message != "" {
		return fmt.Errorf("Azure API 错误 (%s): %s", errEnvelope.Error.Code, errEnvelope.Error.Message)
	}
	return fmt.Errorf("Azure API 返回 HTTP %d: %s", status, truncate(string(raw), 200))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
