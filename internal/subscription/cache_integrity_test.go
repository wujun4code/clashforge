package subscription

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cacheFixtureTransport func(*http.Request) (*http.Response, error)

func (f cacheFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCacheDuplicateMappingWriterPreservesCommit(t *testing.T) {
	for _, raw := range []string{"secret-marker: a\nsecret-marker: b\n", "outer:\n  secret-marker: a\n  secret-marker: b\n"} {
		for _, keepRaw := range []bool{false, true} {
			dir := t.TempDir()
			m := NewManager(dir)
			nodes := []ProxyNode{{Name: "node", Type: "ss"}}
			if err := m.writeCache("fixture", nodes, []byte("rules: [MATCH,DIRECT]\n"), true); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(dir, "cache", "fixture")
			before := map[string]string{}
			for _, suffix := range []string{".json", ".raw.yaml", ".commit.json"} {
				b, err := os.ReadFile(base + suffix)
				if err != nil {
					t.Fatal(err)
				}
				before[suffix] = string(b)
			}
			if err := m.writeCache("fixture", nodes, []byte(raw), keepRaw); err == nil {
				t.Error("invalid mapping downgraded or persisted")
			} else if strings.Contains(err.Error(), "secret-marker") {
				t.Error("key leaked")
			}
			for suffix, want := range before {
				got, err := os.ReadFile(base + suffix)
				if err != nil || string(got) != want {
					t.Errorf("invalid mapping changed %s", suffix)
				}
			}
		}
	}
}

func TestCachePersistentInputCommit(t *testing.T) {
	const full = "proxies:\n - {name: node, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}\nproxy-groups:\n - {name: original, type: select, proxies: [node]}\nrules: [\"MATCH,original\"]\n"
	const list = "- {name: node, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}\n"
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	body := full
	http.DefaultTransport = cacheFixtureTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	for _, mode := range []string{"full", "nodes", "raw-write-failure", "torn-nodes", "torn-raw", "pending", "missing-raw", "corrupt-marker", "legacy-full", "legacy-rawless"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			m := NewManager(dir)
			id, err := m.Add(Subscription{Name: "fixture", URL: "https://offline.invalid/sub", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			body = full
			if mode == "nodes" {
				body = list
			}
			base := filepath.Join(dir, "cache", id)
			if mode == "raw-write-failure" {
				if err := os.MkdirAll(base+".raw.yaml", 0755); err != nil {
					t.Fatal(err)
				}
				if err := m.SyncUpdate(id); err == nil {
					t.Error("raw persistence failure reported success")
				}
				if _, _, err := m.GenerationInput(id); err == nil {
					t.Error("failed commit accepted")
				}
				return
			}
			if err := m.SyncUpdate(id); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "torn-nodes":
				os.WriteFile(base+".json", []byte(`[]`), 0644)
			case "torn-raw":
				os.WriteFile(base+".raw.yaml", []byte("rules: [MATCH,DIRECT]\n"), 0644)
			case "pending":
				os.WriteFile(base+".commit.json", []byte(`{"version":0}`), 0644)
			case "missing-raw":
				os.Remove(base + ".raw.yaml")
			case "corrupt-marker":
				os.WriteFile(base+".commit.json", []byte("{"), 0644)
			case "legacy-full":
				os.Remove(base + ".commit.json")
			case "legacy-rawless":
				os.Remove(base + ".commit.json")
				os.Remove(base + ".raw.yaml")
			}
			nodes, raw, err := m.GenerationInput(id)
			valid := mode == "full" || mode == "nodes" || mode == "legacy-full"
			if valid {
				if err != nil || len(nodes) != 1 {
					t.Fatalf("valid source rejected: nodes=%d err=%v", len(nodes), err)
				}
				if (mode == "nodes") != (len(raw) == 0) {
					t.Error("source format not preserved")
				}
			} else if err == nil {
				t.Error("incomplete generation accepted")
			}
		})
	}
}
