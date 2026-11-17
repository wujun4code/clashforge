package subscription

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawPersistenceFailureReported(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	http.DefaultTransport = rawFailureTransport{}
	dir := t.TempDir()
	m := NewManager(dir)
	id, err := m.Add(Subscription{Name: "fixture", URL: "https://offline.invalid/sub", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "cache", id+".raw.yaml"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.SyncUpdate(id); err == nil {
		t.Fatal("raw persistence failure must not report successful synchronization")
	}
}

type rawFailureTransport struct{}

func (rawFailureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("proxies:\n - {name: node, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}\n"))}, nil
}
