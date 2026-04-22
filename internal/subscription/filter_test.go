package subscription_test

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/subscription"
)

func TestFilter_ExcludeKeyword(t *testing.T) {
	nodes := []subscription.ProxyNode{
		{Name: "香港 01", Server: "hk.example.com", Port: 443},
		{Name: "套餐到期 通知", Server: "notice.example.com", Port: 443},
		{Name: "日本 02", Server: "jp.example.com", Port: 443},
	}
	f := subscription.SubscriptionFilter{Exclude: []string{"套餐"}}
	result := subscription.ApplyFilter(nodes, f)
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes after exclude, got %d", len(result))
	}
	for _, n := range result {
		if n.Name == "套餐到期 通知" {
			t.Error("excluded node should not be in result")
		}
	}
}

func TestFilter_IncludeKeyword(t *testing.T) {
	nodes := []subscription.ProxyNode{
		{Name: "香港 01", Server: "hk.example.com", Port: 443},
		{Name: "日本 02", Server: "jp.example.com", Port: 444},
		{Name: "新加坡 03", Server: "sg.example.com", Port: 445},
	}
	f := subscription.SubscriptionFilter{Include: []string{"香港", "新加坡"}}
	result := subscription.ApplyFilter(nodes, f)
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes after include filter, got %d", len(result))
	}
}

func TestFilter_ExcludeBeatsInclude(t *testing.T) {
	nodes := []subscription.ProxyNode{
		{Name: "香港 套餐到期", Server: "hk.example.com", Port: 443},
	}
	f := subscription.SubscriptionFilter{
		Include: []string{"香港"},
		Exclude: []string{"套餐"},
	}
	result := subscription.ApplyFilter(nodes, f)
	if len(result) != 0 {
		t.Errorf("exclude should beat include, expected 0 nodes, got %d", len(result))
	}
}

func TestFilter_Deduplication(t *testing.T) {
	nodes := []subscription.ProxyNode{
		{Name: "Node A", Server: "1.2.3.4", Port: 443},
		{Name: "Node A dup", Server: "1.2.3.4", Port: 443}, // same server:port
		{Name: "Node B", Server: "5.6.7.8", Port: 443},
	}
	result := subscription.ApplyFilter(nodes, subscription.SubscriptionFilter{})
	if len(result) != 2 {
		t.Fatalf("expected 2 nodes after dedup, got %d", len(result))
	}
	if result[0].Name != "Node A" {
		t.Errorf("expected first of duplicate pair to survive, got %q", result[0].Name)
	}
}

func TestFilter_MaxNodes(t *testing.T) {
	var nodes []subscription.ProxyNode
	for i := 0; i < 10; i++ {
		nodes = append(nodes, subscription.ProxyNode{
			Name: "Node", Server: "different.host.example.com", Port: 1000 + i,
		})
	}
	f := subscription.SubscriptionFilter{MaxNodes: 3}
	result := subscription.ApplyFilter(nodes, f)
	if len(result) != 3 {
		t.Fatalf("expected max 3 nodes, got %d", len(result))
	}
}

func TestFilter_CaseInsensitive(t *testing.T) {
	nodes := []subscription.ProxyNode{
		{Name: "HongKong Premium", Server: "hk.example.com", Port: 443},
	}
	f := subscription.SubscriptionFilter{Include: []string{"hongkong"}}
	result := subscription.ApplyFilter(nodes, f)
	if len(result) != 1 {
		t.Errorf("filter should be case-insensitive, expected 1 node, got %d", len(result))
	}
}

func TestFilter_Empty(t *testing.T) {
	nodes := []subscription.ProxyNode{
		{Name: "Node A", Server: "1.2.3.4", Port: 443},
	}
	result := subscription.ApplyFilter(nodes, subscription.SubscriptionFilter{})
	if len(result) != 1 {
		t.Errorf("empty filter should pass all nodes, got %d", len(result))
	}
}
