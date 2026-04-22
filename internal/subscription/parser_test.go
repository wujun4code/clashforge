package subscription_test

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/subscription"
)

// -------- vmess --------

func TestParseVmess_Basic(t *testing.T) {
	// Real-world vmess link structure (V2RayN format)
	// base64({"v":"2","ps":"test-node","add":"1.2.3.4","port":"443","id":"abcd1234-1234-1234-1234-abcd12345678","aid":"0","scy":"auto","net":"tcp","tls":"tls","sni":"example.com"})
	encoded := "eyJ2IjoiMiIsInBzIjoidGVzdC1ub2RlIiwiYWRkIjoiMS4yLjMuNCIsInBvcnQiOiI0NDMiLCJpZCI6ImFiY2QxMjM0LTEyMzQtMTIzNC0xMjM0LWFiY2QxMjM0NTY3OCIsImFpZCI6IjAiLCJzY3kiOiJhdXRvIiwibmV0IjoidGNwIiwidGxzIjoidGxzIiwic25pIjoiZXhhbXBsZS5jb20ifQ=="
	nodes, err := subscription.Parse([]byte("vmess://" + encoded + "\n"))
	if err != nil {
		t.Fatalf("Parse vmess: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Name != "test-node" {
		t.Errorf("expected name=test-node, got %q", n.Name)
	}
	if n.Server != "1.2.3.4" {
		t.Errorf("expected server=1.2.3.4, got %q", n.Server)
	}
	if n.Port != 443 {
		t.Errorf("expected port=443, got %d", n.Port)
	}
	if n.Extra["uuid"] != "abcd1234-1234-1234-1234-abcd12345678" {
		t.Errorf("expected uuid in Extra, got %v", n.Extra["uuid"])
	}
	if n.Extra["tls"] != true {
		t.Errorf("expected tls=true in Extra")
	}
}

func TestParseVmess_WS(t *testing.T) {
	encoded := "eyJ2IjoiMiIsInBzIjoid3Mtbm9kZSIsImFkZCI6IndzLmV4YW1wbGUuY29tIiwicG9ydCI6NDQzLCJpZCI6InV1aWQtMTIzNCIsImFpZCI6MCwic2N5IjoiYXV0byIsIm5ldCI6IndzIiwiaG9zdCI6IndzLmV4YW1wbGUuY29tIiwicGF0aCI6Ii93cyIsInRscyI6InRscyJ9"
	nodes, err := subscription.Parse([]byte("vmess://" + encoded))
	if err != nil {
		t.Fatalf("Parse vmess ws: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n.Extra["network"] != "ws" {
		t.Errorf("expected network=ws, got %v", n.Extra["network"])
	}
	opts, _ := n.Extra["ws-opts"].(map[string]interface{})
	if opts["path"] != "/ws" {
		t.Errorf("expected ws-opts.path=/ws, got %v", opts["path"])
	}
}

// -------- trojan --------

func TestParseTrojan_Basic(t *testing.T) {
	link := "trojan://mypassword@trojan.example.com:443?sni=trojan.example.com#My%20Node"
	nodes, err := subscription.Parse([]byte(link))
	if err != nil {
		t.Fatalf("Parse trojan: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Name != "My Node" {
		t.Errorf("expected name='My Node', got %q", n.Name)
	}
	if n.Type != "trojan" {
		t.Errorf("expected type=trojan, got %q", n.Type)
	}
	if n.Extra["password"] != "mypassword" {
		t.Errorf("expected password=mypassword, got %v", n.Extra["password"])
	}
	if n.Extra["sni"] != "trojan.example.com" {
		t.Errorf("expected sni, got %v", n.Extra["sni"])
	}
}

func TestParseTrojan_SkipCertVerify(t *testing.T) {
	link := "trojan://pass@example.com:443?allowInsecure=1#insecure"
	nodes, err := subscription.Parse([]byte(link))
	if err != nil {
		t.Fatalf("Parse trojan: %v", err)
	}
	if nodes[0].Extra["skip-cert-verify"] != true {
		t.Error("expected skip-cert-verify=true when allowInsecure=1")
	}
}

// -------- ss --------

func TestParseSS_SIP002(t *testing.T) {
	// ss://BASE64(method:password)@server:port#name
	import_b64 := "YWVzLTI1Ni1nY206c2VjcmV0cGFzc3dvcmQ="
	link := "ss://" + import_b64 + "@ss.example.com:8388#SS-Node"
	nodes, err := subscription.Parse([]byte(link))
	if err != nil {
		t.Fatalf("Parse ss: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n.Type != "ss" {
		t.Errorf("expected type=ss, got %q", n.Type)
	}
	if n.Server != "ss.example.com" {
		t.Errorf("expected server=ss.example.com, got %q", n.Server)
	}
	if n.Port != 8388 {
		t.Errorf("expected port=8388, got %d", n.Port)
	}
}

// -------- vless --------

func TestParseVless_Basic(t *testing.T) {
	link := "vless://myuuid-1234-5678@vless.example.com:443?security=tls&sni=vless.example.com&type=tcp#VLESS-Node"
	nodes, err := subscription.Parse([]byte(link))
	if err != nil {
		t.Fatalf("Parse vless: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}
	n := nodes[0]
	if n.Type != "vless" {
		t.Errorf("expected type=vless, got %q", n.Type)
	}
	if n.Extra["uuid"] != "myuuid-1234-5678" {
		t.Errorf("expected uuid, got %v", n.Extra["uuid"])
	}
	if n.Extra["tls"] != true {
		t.Error("expected tls=true when security=tls")
	}
}

// -------- clash yaml --------

func TestParseClashYAML(t *testing.T) {
	yaml := `proxies:
- name: "node-a"
  type: ss
  server: ss.example.com
  port: 8388
  cipher: aes-256-gcm
  password: testpass
- name: "node-b"
  type: vmess
  server: vmess.example.com
  port: 443
  uuid: some-uuid
  alterId: 0
`
	nodes, err := subscription.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse clash yaml: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes from clash yaml, got %d", len(nodes))
	}
	if nodes[0].Name != "node-a" {
		t.Errorf("expected name=node-a, got %q", nodes[0].Name)
	}
	if nodes[1].Type != "vmess" {
		t.Errorf("expected type=vmess for node-b, got %q", nodes[1].Type)
	}
}

// -------- line-based multi-protocol --------

func TestParseMixedLines(t *testing.T) {
	content := `trojan://pass@t.example.com:443#TJ
vless://uuid@v.example.com:443?security=tls#VL
`
	nodes, err := subscription.Parse([]byte(content))
	if err != nil {
		t.Fatalf("Parse mixed: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	types := map[string]bool{}
	for _, n := range nodes {
		types[n.Type] = true
	}
	if !types["trojan"] {
		t.Error("expected trojan node")
	}
	if !types["vless"] {
		t.Error("expected vless node")
	}
}

// -------- empty / garbage --------

func TestParseEmpty(t *testing.T) {
	nodes, err := subscription.Parse([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes on empty input, got %d", len(nodes))
	}
}

func TestParseGarbage(t *testing.T) {
	nodes, err := subscription.Parse([]byte("this is not a valid subscription content at all!!!"))
	if err != nil {
		t.Fatalf("unexpected error on garbage input: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes on garbage input, got %d", len(nodes))
	}
}
