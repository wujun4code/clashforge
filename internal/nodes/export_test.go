package nodes

import (
	"strings"
	"testing"
)

func TestExportClashProxy_MissingCredentials(t *testing.T) {
	_, err := ExportClashProxy(&Node{
		Name:   "n1",
		Domain: "edge.example.com",
	})
	if err == nil {
		t.Fatal("expected error when proxy credentials are missing")
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

