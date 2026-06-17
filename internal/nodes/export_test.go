package nodes

import (
	"strings"
	"testing"
)

func TestExportClashProxy_EmptyCredentials_Succeeds(t *testing.T) {
	// Credentials are optional — ExportClashProxy only requires a non-empty server address.
	out, err := ExportClashProxy(&Node{
		Name:   "n1",
		Domain: "edge.example.com",
	})
	if err != nil {
		t.Fatalf("ExportClashProxy() unexpected error: %v", err)
	}
	if !strings.Contains(out, "server: edge.example.com") {
		t.Fatalf("expected server in output, got:\n%s", out)
	}
}

func TestExportClashProxy_IncludesPassword(t *testing.T) {
	out, err := ExportClashProxy(&Node{
		Name:          "n1",
		Domain:        "edge.example.com",
		ProxyUser:     "u1",
		ProxyPassword: "p1",
	})
	if err != nil {
		t.Fatalf("ExportClashProxy() error = %v", err)
	}
	if !strings.Contains(out, "username: u1") {
		t.Fatalf("output missing username: %s", out)
	}
	if !strings.Contains(out, "password: p1") {
		t.Fatalf("output missing password: %s", out)
	}
}

// TestExportClashProxy_NilNode ensures nil input returns an error instead of panicking.
func TestExportClashProxy_NilNode(t *testing.T) {
	_, err := ExportClashProxy(nil)
	if err == nil {
		t.Fatal("expected error for nil node")
	}
}

// TestExportClashProxy_UsesDomainAsServer verifies domain is preferred over host.
func TestExportClashProxy_UsesDomainAsServer(t *testing.T) {
	out, err := ExportClashProxy(&Node{
		Name:          "n1",
		Host:          "1.2.3.4",
		Domain:        "edge.example.com",
		ProxyUser:     "u",
		ProxyPassword: "p",
	})
	if err != nil {
		t.Fatalf("ExportClashProxy() error = %v", err)
	}
	if !strings.Contains(out, "server: edge.example.com") {
		t.Fatalf("expected domain as server, got:\n%s", out)
	}
	if strings.Contains(out, "1.2.3.4") {
		t.Fatalf("expected host not to appear when domain is set, got:\n%s", out)
	}
}

// TestExportClashProxy_FallsBackToHost verifies host is used when domain is empty.
func TestExportClashProxy_FallsBackToHost(t *testing.T) {
	out, err := ExportClashProxy(&Node{
		Name:          "n2",
		Host:          "10.0.0.1",
		Domain:        "",
		ProxyUser:     "u",
		ProxyPassword: "p",
	})
	if err != nil {
		t.Fatalf("ExportClashProxy() error = %v", err)
	}
	if !strings.Contains(out, "server: 10.0.0.1") {
		t.Fatalf("expected host as server fallback, got:\n%s", out)
	}
}

// TestExportClashProxy_EmptyHostAndDomain returns an error when neither domain nor host is set.
func TestExportClashProxy_EmptyHostAndDomain(t *testing.T) {
	_, err := ExportClashProxy(&Node{
		Name:          "n3",
		Host:          "",
		Domain:        "",
		ProxyUser:     "u",
		ProxyPassword: "p",
	})
	if err == nil {
		t.Fatal("expected error when both host and domain are empty")
	}
}

// TestExportClashProxy_TLSEnabled verifies TLS is enabled in the generated config.
func TestExportClashProxy_TLSEnabled(t *testing.T) {
	out, err := ExportClashProxy(&Node{
		Name:          "n4",
		Domain:        "edge.example.com",
		ProxyUser:     "u",
		ProxyPassword: "p",
	})
	if err != nil {
		t.Fatalf("ExportClashProxy() error = %v", err)
	}
	if !strings.Contains(out, "tls: true") {
		t.Fatalf("expected tls: true in output, got:\n%s", out)
	}
}

