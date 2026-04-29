package workernode

import (
	"strings"
	"testing"
)

func TestExportClashProxy_IncludesRequiredFields(t *testing.T) {
	out, err := ExportClashProxy(&WorkerNode{
		Name:       "cf-edge-1",
		Hostname:   "edge.example.com",
		WorkerUUID: "7ed0d7f0-d950-4df0-80ef-2272e2fb1ff4",
	})
	if err != nil {
		t.Fatalf("ExportClashProxy() error = %v", err)
	}

	expectContains := []string{
		"proxies:",
		"- name: cf-edge-1",
		"type: vless",
		"server: edge.example.com",
		"uuid: 7ed0d7f0-d950-4df0-80ef-2272e2fb1ff4",
		"network: ws",
		"Host: edge.example.com",
	}
	for _, needle := range expectContains {
		if !strings.Contains(out, needle) {
			t.Fatalf("output missing %q:\n%s", needle, out)
		}
	}
}

func TestExportClashProxy_MissingHostname(t *testing.T) {
	_, err := ExportClashProxy(&WorkerNode{
		Name:       "cf-edge-1",
		WorkerUUID: "7ed0d7f0-d950-4df0-80ef-2272e2fb1ff4",
	})
	if err == nil {
		t.Fatal("expected error when hostname is empty")
	}
}

func TestExportClashProxy_MissingUUID(t *testing.T) {
	_, err := ExportClashProxy(&WorkerNode{
		Name:     "cf-edge-1",
		Hostname: "edge.example.com",
	})
	if err == nil {
		t.Fatal("expected error when uuid is empty")
	}
}
