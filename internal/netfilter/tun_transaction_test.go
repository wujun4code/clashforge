package netfilter

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func updateTunMock(t *testing.T, changes map[string]any) {
	t.Helper()
	p := os.Getenv("TUN_MOCK_STATE")
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	var s map[string]any
	if e = json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	for k, v := range changes {
		s[k] = v
	}
	b, e = json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
}

var ownedTunRules = []string{"9020: from all lookup main suppress_prefixlength 0 proto 242", "9021: from all lookup 2022 proto 242"}

const ownedTunForward = `meta nfproto ipv4 oifname "Meta" counter packets 3 bytes 120 accept comment "clashforge: allow forwarding into TUN device" # handle 42`

func TestTUNApplyIdempotentAndCleanup(t *testing.T) {
	read := tunMock(t, nil, nil, "")
	m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
	for i := 0; i < 2; i++ {
		if err := m.Apply(); err != nil {
			t.Fatal(err)
		}
		if !m.IsApplied() {
			t.Fatal("successful apply not marked ready")
		}
	}
	s := read()
	if len(s["rules"].([]any)) != 2 || len(s["nft"].([]any)) != 1 {
		t.Fatalf("apply not idempotent: %v", s)
	}
	if err := m.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if m.IsApplied() {
		t.Fatal("still applied after cleanup")
	}
	s = read()
	if len(s["rules"].([]any)) != 0 || len(s["nft"].([]any)) != 0 {
		t.Fatalf("owned state remains: %v", s)
	}
	if err := m.Cleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestTUNRollbackOnlyNewResources(t *testing.T) {
	for _, preexisting := range []bool{false, true} {
		t.Run(map[bool]string{false: "all_new", true: "preexisting_forward_and_first_rule"}[preexisting], func(t *testing.T) {
			var rules, nft []string
			if preexisting {
				rules = ownedTunRules[:1]
				nft = []string{ownedTunForward}
			}
			read := tunMock(t, rules, nft, "rule add pref 9021")
			m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
			if err := m.Apply(); err == nil || m.IsApplied() {
				t.Fatalf("failed second acquisition reported success: %v", err)
			}
			s := read()
			want := 0
			if preexisting {
				want = 1
			}
			if len(s["rules"].([]any)) != want || len(s["nft"].([]any)) != want {
				t.Fatalf("rollback removed preexisting or retained new state: %v", s)
			}
		})
	}
}

func TestTUNRollbackFailureRetriedByCleanup(t *testing.T) {
	read := tunMock(t, nil, nil, "")
	updateTunMock(t, map[string]any{"fail": []string{"rule add pref 9021", "rule del", "nft delete"}})
	m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
	err := m.Apply()
	if err == nil || !strings.Contains(err.Error(), "rollback newly added TUN routes") || !strings.Contains(err.Error(), "rollback newly added TUN forward") || m.IsApplied() {
		t.Fatalf("lost apply/rollback errors: %v", err)
	}
	if err := m.Apply(); err == nil || !strings.Contains(err.Error(), "unfinished") {
		t.Fatalf("reapply accepted unfinished rollback: %v", err)
	}
	updateTunMock(t, map[string]any{"fail": ""})
	if err := m.Cleanup(); err != nil {
		t.Fatal(err)
	}
	s := read()
	if len(s["rules"].([]any)) != 0 || len(s["nft"].([]any)) != 0 {
		t.Fatalf("failed apply could not be cleaned: %v", s)
	}
}

func TestTUNCleanupFailureRetainsOwnershipForRetry(t *testing.T) {
	read := tunMock(t, nil, nil, "")
	m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
	if err := m.Apply(); err != nil {
		t.Fatal(err)
	}
	updateTunMock(t, map[string]any{"fail": "nft delete"})
	if err := m.Cleanup(); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	if m.IsApplied() {
		t.Fatal("partly cleaned rules still marked applied")
	}
	updateTunMock(t, map[string]any{"fail": ""})
	if err := m.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if len(read()["nft"].([]any)) != 0 {
		t.Fatal("retry lost ownership")
	}
}

func TestTUNRejectsIncompleteSemantics(t *testing.T) {
	cases := [][]string{
		{"9020: from all lookup main suppress_prefixlength 1 proto 242"},
		{"9020: from all fwmark 0x1 lookup main suppress_prefixlength 0 proto 242"},
		{"9021: from 192.168.1.0/24 lookup 2022 proto 242"},
		{"9021: from all lookup 999 proto 242"},
		{"9021: from all lookup 2022"},
		{ownedTunRules[0], ownedTunRules[0]},
		{ownedTunRules[0], ownedTunRules[1], "9021: from all lookup 999"},
	}
	for _, rules := range cases {
		t.Run(strings.Join(rules, ";"), func(t *testing.T) {
			read := tunMock(t, rules, nil, "")
			if err := EnsureTunRouteRule(); err == nil {
				t.Fatal("accepted incompatible or duplicate occupants")
			}
			for _, v := range read()["calls"].([]any) {
				if strings.Contains(v.(string), "rule add") || strings.Contains(v.(string), "rule del") {
					t.Fatalf("mutated conflicting state: %s", v)
				}
			}
		})
	}
}

func TestTUNForeignSelectorsNotDeleted(t *testing.T) {
	foreign := []string{"9020: from all fwmark 0x1 lookup main suppress_prefixlength 0 proto 242", "9021: from all to 203.0.113.0/24 lookup 2022 proto 242", "9022: from all lookup 2022 proto 242"}
	read := tunMock(t, foreign, nil, "")
	if err := RemoveTunRouteRule(); err != nil {
		t.Fatal(err)
	}
	if len(read()["rules"].([]any)) != len(foreign) {
		t.Fatal("deleted foreign selector or priority")
	}
}

func TestTUNCleanupRefusesAmbiguousPriority(t *testing.T) {
	read := tunMock(t, []string{ownedTunRules[1], "9021: from all to 203.0.113.0/24 lookup 2022 proto 242"}, nil, "")
	if err := RemoveTunRouteRule(); err == nil {
		t.Fatal("ambiguous cleanup should be reported")
	}
	if len(read()["rules"].([]any)) != 2 {
		t.Fatal("ambiguous foreign rule deleted")
	}
}

func TestTUNForwardOwnershipAndDevice(t *testing.T) {
	foreign := `meta nfproto ipv4 oifname "Meta" accept comment "user rule" # handle 43`
	other := strings.ReplaceAll(strings.ReplaceAll(ownedTunForward, `"Meta"`, `"Other"`), "handle 42", "handle 44")
	read := tunMock(t, nil, []string{ownedTunForward, foreign, other}, "")
	if err := RemoveTunForwardAccept("Meta"); err != nil {
		t.Fatal(err)
	}
	s := read()
	if len(s["nft"].([]any)) != 2 {
		t.Fatalf("removed foreign comment/device: %v", s)
	}
}

func TestTUNForwardForeignDropIsNotAcceptedAsReady(t *testing.T) {
	foreign := `oifname "Meta" drop comment "user rule" # handle 43`
	read := tunMock(t, nil, []string{foreign}, "")
	if err := EnsureTunForwardAccept("Meta"); err != nil {
		t.Fatal(err)
	}
	if len(read()["nft"].([]any)) != 2 {
		t.Fatal("foreign oifname match incorrectly treated as owned accept")
	}
}

func TestTUNUnsupportedScopeRejected(t *testing.T) {
	for _, cfg := range []Config{
		{Mode: "tun", FirewallBackend: "nftables", EnableIPv6: true},
		{Mode: "tun", FirewallBackend: "none"},
		{Mode: "tun", FirewallBackend: "iptables"},
		{Mode: "tun", FirewallBackend: "nftables", TunDevice: "bad\"device"},
	} {
		t.Run(cfg.FirewallBackend+cfg.TunDevice, func(t *testing.T) {
			read := tunMock(t, nil, nil, "")
			m := NewManager(cfg)
			if err := m.Apply(); err == nil || m.IsApplied() {
				t.Fatal("unsupported scope falsely applied")
			}
			if len(read()["calls"].([]any)) != 0 {
				t.Fatal("unsupported scope mutated/inspected networking")
			}
		})
	}
}

func TestTUNPrerequisitesAndInspectionFailures(t *testing.T) {
	for _, failure := range []string{"ip link show", "ip route show", "ip rule show", "nft -a list"} {
		t.Run(failure, func(t *testing.T) {
			read := tunMock(t, nil, nil, failure)
			m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
			if err := m.Apply(); err == nil || m.IsApplied() {
				t.Fatal("inspection failure reported success")
			}
			if len(read()["nft"].([]any)) != 0 {
				t.Fatal("failed inspection left newly created rule")
			}
		})
	}
}

func TestTUNRejectsMissingOrWrongDefault(t *testing.T) {
	for _, route := range []string{"", "default via 192.0.2.1 dev eth0", "default dev Meta linkdown", "default dev Meta\ndefault dev eth0", "default nexthop dev Meta nexthop dev eth0"} {
		t.Run(route, func(t *testing.T) {
			read := tunMock(t, nil, nil, "")
			updateTunMock(t, map[string]any{"route": route})
			m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
			if err := m.Apply(); err == nil || m.IsApplied() {
				t.Fatal("invalid TUN table falsely ready")
			}
			if len(read()["nft"].([]any)) != 0 {
				t.Fatal("changed firewall before route readiness")
			}
		})
	}
}

func TestTUNSuccessWithoutStateIsRejected(t *testing.T) {
	for _, noop := range []string{"nft insert", "ip rule add"} {
		t.Run(noop, func(t *testing.T) {
			tunMock(t, nil, nil, "")
			updateTunMock(t, map[string]any{"noop": []string{noop}})
			m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
			if err := m.Apply(); err == nil || m.IsApplied() {
				t.Fatal("exit 0 without required state falsely succeeded")
			}
		})
	}
}

func TestTUNAdditionalCleanupAndLegacyBoundaries(t *testing.T) {
	t.Run("inspection_errors", func(t *testing.T) {
		tunMock(t, nil, nil, "ip rule show")
		if err := RemoveTunRouteRule(); err == nil {
			t.Fatal("rule inspection error hidden")
		}
		updateTunMock(t, map[string]any{"fail": "nft -a list"})
		if err := RemoveTunForwardAccept("Meta"); err == nil {
			t.Fatal("fw4 inspection error hidden")
		}
	})
	t.Run("successful_noop_deletion", func(t *testing.T) {
		tunMock(t, ownedTunRules, []string{ownedTunForward}, "")
		updateTunMock(t, map[string]any{"noop": []string{"rule del", "nft delete"}})
		if err := RemoveTunRouteRule(); err == nil {
			t.Fatal("rules remain despite delete exit 0")
		}
		if err := RemoveTunForwardAccept("Meta"); err == nil {
			t.Fatal("forward rule remains despite delete exit 0")
		}
	})
	t.Run("legacy_owned_forward", func(t *testing.T) {
		legacy := strings.TrimPrefix(ownedTunForward, "meta nfproto ipv4 ")
		read := tunMock(t, nil, []string{legacy}, "")
		if err := EnsureTunForwardAccept("Meta"); err == nil {
			t.Fatal("adopted legacy all-family rule as IPv4-only")
		}
		if err := RemoveTunForwardAccept("Meta"); err != nil {
			t.Fatal(err)
		}
		if len(read()["nft"].([]any)) != 0 {
			t.Fatal("explicitly owned legacy rule not cleaned")
		}
	})
	t.Run("incompatible_owned_forward", func(t *testing.T) {
		read := tunMock(t, nil, []string{strings.Replace(ownedTunForward, "accept comment", "drop comment", 1)}, "")
		if err := RemoveTunForwardAccept("Meta"); err == nil {
			t.Fatal("incompatible owned rule cleanup silently skipped")
		}
		if len(read()["nft"].([]any)) != 1 {
			t.Fatal("unsafe deletion of incompatible rule")
		}
	})
	t.Run("down_device", func(t *testing.T) {
		tunMock(t, nil, nil, "")
		updateTunMock(t, map[string]any{"link": "9: Meta: <POINTOPOINT> mtu 1500"})
		m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
		if err := m.Apply(); err == nil || m.IsApplied() {
			t.Fatal("down device reported ready")
		}
	})
}

func TestTUNStandaloneRouteAcquisitionRollsBack(t *testing.T) {
	read := tunMock(t, nil, nil, "rule add pref 9021")
	if err := EnsureTunRouteRule(); err == nil {
		t.Fatal("second route failure swallowed")
	}
	if len(read()["rules"].([]any)) != 0 {
		t.Fatal("standalone route helper left partial pair")
	}
}
