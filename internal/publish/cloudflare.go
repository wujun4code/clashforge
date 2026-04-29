package publish

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

type CloudflareClient struct {
	token  string
	client *http.Client
}

type cloudflareErrorItem struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cloudflareEnvelope[T any] struct {
	Success bool                  `json:"success"`
	Result  T                     `json:"result"`
	Errors  []cloudflareErrorItem `json:"errors"`
}

type cloudflarePageInfo struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

type namespaceListResponse struct {
	Success    bool                  `json:"success"`
	Result     []cloudflareNamespace `json:"result"`
	ResultInfo cloudflarePageInfo    `json:"result_info"`
	Errors     []cloudflareErrorItem `json:"errors"`
}

type cloudflareNamespace struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type workersSubdomainResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Subdomain string `json:"subdomain"`
	} `json:"result"`
	Errors []cloudflareErrorItem `json:"errors"`
}

func NewCloudflareClient(token string) (*CloudflareClient, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("cloudflare token is required")
	}
	return &CloudflareClient{
		token: token,
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

func (c *CloudflareClient) CheckPermissions(ctx context.Context, accountID, zoneID string) []PermissionCheck {
	checks := make([]PermissionCheck, 0, 3)

	checks = append(checks, c.checkAccess(ctx,
		"Workers KV Storage:Edit",
		fmt.Sprintf("/accounts/%s/storage/kv/namespaces?per_page=1", url.PathEscape(strings.TrimSpace(accountID))),
	))
	checks = append(checks, c.checkAccess(ctx,
		"Workers Scripts:Edit",
		fmt.Sprintf("/accounts/%s/workers/scripts", url.PathEscape(strings.TrimSpace(accountID))),
	))
	if strings.TrimSpace(zoneID) != "" {
		checks = append(checks, c.checkAccess(ctx,
			"Zone:Read",
			fmt.Sprintf("/zones/%s", url.PathEscape(strings.TrimSpace(zoneID))),
		))
	}
	return checks
}

func (c *CloudflareClient) checkAccess(ctx context.Context, name, path string) PermissionCheck {
	_, status, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return PermissionCheck{Name: name, OK: false, Error: err.Error()}
	}
	if status < 200 || status >= 300 {
		return PermissionCheck{Name: name, OK: false, Error: fmt.Sprintf("unexpected status %d", status)}
	}
	return PermissionCheck{Name: name, OK: true}
}

func (c *CloudflareClient) CreateOrReuseNamespace(ctx context.Context, accountID, workerName string) (WorkerNamespaceResult, error) {
	accountID = strings.TrimSpace(accountID)
	workerName = strings.TrimSpace(workerName)
	if accountID == "" {
		return WorkerNamespaceResult{}, fmt.Errorf("account_id is required")
	}
	if workerName == "" {
		return WorkerNamespaceResult{}, fmt.Errorf("worker_name is required")
	}

	title := "kv-" + workerName
	page := 1
	for {
		path := fmt.Sprintf("/accounts/%s/storage/kv/namespaces?page=%d&per_page=100", url.PathEscape(accountID), page)
		body, status, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
		if err != nil {
			return WorkerNamespaceResult{}, err
		}
		if status < 200 || status >= 300 {
			return WorkerNamespaceResult{}, c.parseStatusError(status, body)
		}

		var list namespaceListResponse
		if err := json.Unmarshal(body, &list); err != nil {
			return WorkerNamespaceResult{}, fmt.Errorf("parse namespace list: %w", err)
		}
		if !list.Success {
			return WorkerNamespaceResult{}, c.parseEnvelopeError(list.Errors, "cloudflare namespace list failed")
		}
		for _, item := range list.Result {
			if strings.TrimSpace(item.Title) == title {
				return WorkerNamespaceResult{
					NamespaceID: item.ID,
					Reused:      true,
					Title:       title,
				}, nil
			}
		}
		if list.ResultInfo.TotalPages <= 0 || page >= list.ResultInfo.TotalPages {
			break
		}
		page++
	}

	payload := map[string]string{"title": title}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkerNamespaceResult{}, err
	}
	body, status, err := c.doRequest(ctx, http.MethodPost,
		fmt.Sprintf("/accounts/%s/storage/kv/namespaces", url.PathEscape(accountID)),
		bytes.NewReader(raw),
		"application/json",
	)
	if err != nil {
		return WorkerNamespaceResult{}, err
	}
	if status < 200 || status >= 300 {
		return WorkerNamespaceResult{}, c.parseStatusError(status, body)
	}

	var resp cloudflareEnvelope[cloudflareNamespace]
	if err := json.Unmarshal(body, &resp); err != nil {
		return WorkerNamespaceResult{}, fmt.Errorf("parse namespace create response: %w", err)
	}
	if !resp.Success {
		return WorkerNamespaceResult{}, c.parseEnvelopeError(resp.Errors, "cloudflare create namespace failed")
	}
	return WorkerNamespaceResult{
		NamespaceID: resp.Result.ID,
		Reused:      false,
		Title:       title,
	}, nil
}

func (c *CloudflareClient) DeployWorkerScript(
	ctx context.Context,
	accountID, workerName, namespaceID, accessToken string,
) (WorkerDeployResult, error) {
	accountID = strings.TrimSpace(accountID)
	workerName = strings.TrimSpace(workerName)
	namespaceID = strings.TrimSpace(namespaceID)
	accessToken = strings.TrimSpace(accessToken)

	if accountID == "" || workerName == "" || namespaceID == "" || accessToken == "" {
		return WorkerDeployResult{}, fmt.Errorf("account_id, worker_name, namespace_id and access_token are required")
	}

	metadata := map[string]any{
		"main_module": "worker.js",
		"bindings": []map[string]string{
			{
				"type":         "kv_namespace",
				"name":         "KV",
				"namespace_id": namespaceID,
			},
			{
				"type": "plain_text",
				"name": "TOKEN",
				"text": accessToken,
			},
		},
	}
	metaRaw, err := json.Marshal(metadata)
	if err != nil {
		return WorkerDeployResult{}, err
	}

	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)

	metaHeader := textproto.MIMEHeader{}
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"; filename="blob"`)
	metaHeader.Set("Content-Type", "application/json")
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return WorkerDeployResult{}, err
	}
	if _, err := metaPart.Write(metaRaw); err != nil {
		return WorkerDeployResult{}, err
	}

	scriptHeader := textproto.MIMEHeader{}
	scriptHeader.Set("Content-Disposition", `form-data; name="worker.js"; filename="worker.js"`)
	scriptHeader.Set("Content-Type", "application/javascript+module")
	scriptPart, err := writer.CreatePart(scriptHeader)
	if err != nil {
		return WorkerDeployResult{}, err
	}
	if _, err := io.WriteString(scriptPart, WorkerScriptSource); err != nil {
		return WorkerDeployResult{}, err
	}

	if err := writer.Close(); err != nil {
		return WorkerDeployResult{}, err
	}

	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s",
		url.PathEscape(accountID),
		url.PathEscape(workerName),
	)
	body, status, err := c.doRequest(ctx, http.MethodPut, path, &formBody, writer.FormDataContentType())
	if err != nil {
		return WorkerDeployResult{}, err
	}
	if status < 200 || status >= 300 {
		return WorkerDeployResult{}, c.parseStatusError(status, body)
	}
	var uploadResp cloudflareEnvelope[json.RawMessage]
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return WorkerDeployResult{}, fmt.Errorf("parse worker upload response: %w", err)
	}
	if !uploadResp.Success {
		return WorkerDeployResult{}, c.parseEnvelopeError(uploadResp.Errors, "cloudflare upload worker failed")
	}

	workersSubdomain := ""
	subPath := fmt.Sprintf("/accounts/%s/workers/subdomain", url.PathEscape(accountID))
	subBody, subStatus, subErr := c.doRequest(ctx, http.MethodGet, subPath, nil, "")
	if subErr == nil && subStatus >= 200 && subStatus < 300 {
		var subResp workersSubdomainResponse
		if err := json.Unmarshal(subBody, &subResp); err == nil && subResp.Success {
			workersSubdomain = strings.TrimSpace(subResp.Result.Subdomain)
		}
	}

	devURL := fmt.Sprintf("https://%s.workers.dev", workerName)
	if workersSubdomain != "" {
		devURL = fmt.Sprintf("https://%s.%s.workers.dev", workerName, workersSubdomain)
	}
	return WorkerDeployResult{
		WorkerDevURL:     devURL,
		WorkersSubdomain: workersSubdomain,
	}, nil
}

// DeployRawWorkerScript uploads a Worker script with arbitrary plain_text bindings.
func (c *CloudflareClient) DeployRawWorkerScript(
	ctx context.Context,
	accountID, workerName, scriptSource string,
	plainBindings map[string]string,
) (WorkerDeployResult, error) {
	accountID = strings.TrimSpace(accountID)
	workerName = strings.TrimSpace(workerName)
	if accountID == "" || workerName == "" {
		return WorkerDeployResult{}, fmt.Errorf("account_id and worker_name are required")
	}

	bindings := make([]map[string]string, 0, len(plainBindings))
	for k, v := range plainBindings {
		bindings = append(bindings, map[string]string{"type": "plain_text", "name": k, "text": v})
	}
	metadata := map[string]any{"main_module": "worker.js", "bindings": bindings}
	metaRaw, err := json.Marshal(metadata)
	if err != nil {
		return WorkerDeployResult{}, err
	}

	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)

	metaHeader := textproto.MIMEHeader{}
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"; filename="blob"`)
	metaHeader.Set("Content-Type", "application/json")
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return WorkerDeployResult{}, err
	}
	if _, err := metaPart.Write(metaRaw); err != nil {
		return WorkerDeployResult{}, err
	}

	scriptHeader := textproto.MIMEHeader{}
	scriptHeader.Set("Content-Disposition", `form-data; name="worker.js"; filename="worker.js"`)
	scriptHeader.Set("Content-Type", "application/javascript+module")
	scriptPart, err := writer.CreatePart(scriptHeader)
	if err != nil {
		return WorkerDeployResult{}, err
	}
	if _, err := io.WriteString(scriptPart, scriptSource); err != nil {
		return WorkerDeployResult{}, err
	}
	if err := writer.Close(); err != nil {
		return WorkerDeployResult{}, err
	}

	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s",
		url.PathEscape(accountID), url.PathEscape(workerName))
	body, status, err := c.doRequest(ctx, http.MethodPut, path, &formBody, writer.FormDataContentType())
	if err != nil {
		return WorkerDeployResult{}, err
	}
	if status < 200 || status >= 300 {
		return WorkerDeployResult{}, c.parseStatusError(status, body)
	}
	var uploadResp cloudflareEnvelope[json.RawMessage]
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return WorkerDeployResult{}, fmt.Errorf("parse worker upload response: %w", err)
	}
	if !uploadResp.Success {
		return WorkerDeployResult{}, c.parseEnvelopeError(uploadResp.Errors, "cloudflare upload worker failed")
	}

	workersSubdomain := ""
	subPath := fmt.Sprintf("/accounts/%s/workers/subdomain", url.PathEscape(accountID))
	subBody, subStatus, subErr := c.doRequest(ctx, http.MethodGet, subPath, nil, "")
	if subErr == nil && subStatus >= 200 && subStatus < 300 {
		var subResp workersSubdomainResponse
		if err := json.Unmarshal(subBody, &subResp); err == nil && subResp.Success {
			workersSubdomain = strings.TrimSpace(subResp.Result.Subdomain)
		}
	}
	devURL := fmt.Sprintf("https://%s.workers.dev", workerName)
	if workersSubdomain != "" {
		devURL = fmt.Sprintf("https://%s.%s.workers.dev", workerName, workersSubdomain)
	}
	return WorkerDeployResult{WorkerDevURL: devURL, WorkersSubdomain: workersSubdomain}, nil
}

// DeleteWorkerScript removes a Worker script from CF.
func (c *CloudflareClient) DeleteWorkerScript(ctx context.Context, accountID, workerName string) error {
	accountID = strings.TrimSpace(accountID)
	workerName = strings.TrimSpace(workerName)
	if accountID == "" || workerName == "" {
		return fmt.Errorf("account_id and worker_name are required")
	}
	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s",
		url.PathEscape(accountID), url.PathEscape(workerName))
	body, status, err := c.doRequest(ctx, http.MethodDelete, path, nil, "")
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status >= 300 {
		return c.parseStatusError(status, body)
	}
	return nil
}

func (c *CloudflareClient) BindWorkerDomain(ctx context.Context, accountID, zoneID, workerName, hostname string) (WorkerBindResult, error) {
	accountID = strings.TrimSpace(accountID)
	zoneID = strings.TrimSpace(zoneID)
	workerName = strings.TrimSpace(workerName)
	hostname = strings.TrimSpace(hostname)

	if accountID == "" || zoneID == "" || workerName == "" || hostname == "" {
		return WorkerBindResult{}, fmt.Errorf("account_id, zone_id, worker_name and hostname are required")
	}

	payload := map[string]any{
		"environment":                  "production",
		"hostname":                     hostname,
		"service":                      workerName,
		"zone_id":                      zoneID,
		"override_existing_dns_record": true,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkerBindResult{}, err
	}

	path := fmt.Sprintf("/accounts/%s/workers/domains", url.PathEscape(accountID))
	body, status, err := c.doRequest(ctx, http.MethodPut, path, bytes.NewReader(raw), "application/json")
	if err != nil {
		return WorkerBindResult{}, err
	}
	if status < 200 || status >= 300 {
		return WorkerBindResult{}, c.parseStatusError(status, body)
	}
	var resp cloudflareEnvelope[json.RawMessage]
	if err := json.Unmarshal(body, &resp); err != nil {
		return WorkerBindResult{}, fmt.Errorf("parse domain bind response: %w", err)
	}
	if !resp.Success {
		return WorkerBindResult{}, c.parseEnvelopeError(resp.Errors, "cloudflare bind domain failed")
	}
	return WorkerBindResult{
		Hostname:  hostname,
		WorkerURL: "https://" + hostname,
	}, nil
}

func VerifyWorkerEndpoint(
	ctx context.Context,
	workerURL, workerDevURL, accessToken string,
) (WorkerVerifyResult, error) {
	workerURL = strings.TrimSpace(workerURL)
	workerDevURL = strings.TrimSpace(workerDevURL)
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return WorkerVerifyResult{}, fmt.Errorf("access_token is required")
	}

	candidates := make([]string, 0, 2)
	addCandidate := func(v string) {
		v = strings.TrimRight(strings.TrimSpace(v), "/")
		if v == "" {
			return
		}
		for _, item := range candidates {
			if item == v {
				return
			}
		}
		candidates = append(candidates, v)
	}
	addCandidate(workerURL)
	addCandidate(workerDevURL)
	if len(candidates) == 0 {
		return WorkerVerifyResult{}, fmt.Errorf("worker url is required")
	}

	client := &http.Client{Timeout: 18 * time.Second}
	testKey := fmt.Sprintf("hello-%d.txt", time.Now().UnixNano())
	testContent := "Hello from clashforge at " + time.Now().UTC().Format(time.RFC3339)

	tests := make([]VerifyTest, 0, 3)
	writeOK := false
	usedURL := ""
	lastErr := ""

	for attempt := 0; attempt < 3 && !writeOK; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return WorkerVerifyResult{}, ctx.Err()
			case <-time.After(3 * time.Second):
			}
		}
		for _, base := range candidates {
			status, body, err := workerWriteByQuery(ctx, client, base, testKey, accessToken, testContent)
			if err != nil {
				lastErr = fmt.Sprintf("%s -> %v", base, err)
				continue
			}
			if status == http.StatusOK {
				writeOK = true
				usedURL = base
				break
			}
			lastErr = fmt.Sprintf("%s -> HTTP %d: %s", base, status, trimForError(body))
		}
	}
	tests = append(tests, VerifyTest{
		Name:   "写入测试",
		OK:     writeOK,
		Detail: pickDetail(writeOK, "已通过 "+usedURL, "两个 URL 均写入失败："+lastErr),
	})

	helloURL := ""
	if writeOK {
		status, body, err := workerRead(ctx, client, usedURL, testKey, accessToken)
		ok := err == nil && status == http.StatusOK && strings.Contains(body, "Hello from clashforge")
		detail := ""
		if !ok {
			if err != nil {
				detail = err.Error()
			} else {
				detail = fmt.Sprintf("HTTP %d: %s", status, trimForError(body))
			}
		}
		tests = append(tests, VerifyTest{Name: "读取验证", OK: ok, Detail: detail})
		helloURL = fmt.Sprintf("%s/%s?token=%s", usedURL, url.PathEscape(testKey), url.QueryEscape(accessToken))

		authStatus, authBody, authErr := workerRead(ctx, client, usedURL, testKey, "")
		authOK := authErr == nil && authStatus == http.StatusForbidden
		authDetail := ""
		if !authOK {
			if authErr != nil {
				authDetail = authErr.Error()
			} else {
				authDetail = fmt.Sprintf("expected 403, got %d: %s", authStatus, trimForError(authBody))
			}
		}
		tests = append(tests, VerifyTest{
			Name:   "鉴权验证（无 token 应返回 403）",
			OK:     authOK,
			Detail: authDetail,
		})
	} else {
		tests = append(tests, VerifyTest{Name: "读取验证", OK: false, Detail: "写入失败，跳过读取验证"})
		tests = append(tests, VerifyTest{Name: "鉴权验证（无 token 应返回 403）", OK: false, Detail: "写入失败，跳过鉴权验证"})
	}

	allOK := true
	for _, test := range tests {
		if !test.OK {
			allOK = false
			break
		}
	}

	result := WorkerVerifyResult{
		OK:       allOK,
		Tests:    tests,
		UsedURL:  usedURL,
		HelloURL: helloURL,
	}
	if allOK && workerURL != "" && usedURL != strings.TrimRight(workerURL, "/") {
		result.Note = "自定义域名可能尚未完成 DNS 传播，已通过 workers.dev 完成验证。"
	}
	return result, nil
}

func UploadContentViaWorker(ctx context.Context, workerBaseURL, fileName, accessToken, content string) error {
	workerBaseURL = strings.TrimRight(strings.TrimSpace(workerBaseURL), "/")
	fileName = strings.TrimSpace(fileName)
	accessToken = strings.TrimSpace(accessToken)
	if workerBaseURL == "" || fileName == "" || accessToken == "" {
		return fmt.Errorf("worker url, file name and access token are required")
	}

	u, err := url.Parse(workerBaseURL + "/" + url.PathEscape(fileName))
	if err != nil {
		return err
	}
	query := u.Query()
	query.Set("token", accessToken)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed, status %d: %s", resp.StatusCode, trimForError(string(body)))
	}
	return nil
}

func DeleteContentViaWorker(ctx context.Context, workerBaseURL, fileName, accessToken string) error {
	workerBaseURL = strings.TrimRight(strings.TrimSpace(workerBaseURL), "/")
	fileName = strings.TrimSpace(fileName)
	accessToken = strings.TrimSpace(accessToken)
	if workerBaseURL == "" || fileName == "" || accessToken == "" {
		return fmt.Errorf("worker url, file name and access token are required")
	}

	u, err := url.Parse(workerBaseURL + "/" + url.PathEscape(fileName))
	if err != nil {
		return err
	}
	query := u.Query()
	query.Set("token", accessToken)
	query.Set("delete", "1")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete failed, status %d: %s", resp.StatusCode, trimForError(string(body)))
	}
	return nil
}

func (c *CloudflareClient) doRequest(
	ctx context.Context,
	method, path string,
	body io.Reader,
	contentType string,
) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, cloudflareAPIBase+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func (c *CloudflareClient) parseStatusError(status int, body []byte) error {
	if len(body) == 0 {
		return fmt.Errorf("cloudflare api status %d", status)
	}
	var env cloudflareEnvelope[json.RawMessage]
	if err := json.Unmarshal(body, &env); err == nil {
		if len(env.Errors) > 0 {
			return c.parseEnvelopeError(env.Errors, fmt.Sprintf("cloudflare api status %d", status))
		}
	}
	return fmt.Errorf("cloudflare api status %d: %s", status, trimForError(string(body)))
}

func (c *CloudflareClient) parseEnvelopeError(errors []cloudflareErrorItem, fallback string) error {
	if len(errors) == 0 {
		return fmt.Errorf("%s", fallback)
	}
	msgs := make([]string, 0, len(errors))
	for _, item := range errors {
		if item.Code > 0 {
			msgs = append(msgs, fmt.Sprintf("[%d] %s", item.Code, item.Message))
		} else {
			msgs = append(msgs, item.Message)
		}
	}
	return fmt.Errorf(strings.Join(msgs, "; "))
}

func workerWriteByQuery(ctx context.Context, client *http.Client, baseURL, key, token, text string) (int, string, error) {
	if client == nil {
		client = &http.Client{Timeout: 18 * time.Second}
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(key))
	if err != nil {
		return 0, "", err
	}
	query := u.Query()
	query.Set("token", token)
	query.Set("b64", b64)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func workerRead(ctx context.Context, client *http.Client, baseURL, key, token string) (int, string, error) {
	if client == nil {
		client = &http.Client{Timeout: 18 * time.Second}
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(key))
	if err != nil {
		return 0, "", err
	}
	query := u.Query()
	if token != "" {
		query.Set("token", token)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func pickDetail(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func trimForError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 220 {
		return value
	}
	return value[:220] + "..."
}
