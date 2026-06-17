package api

import (
	"net/http"
	"os/exec"
	"strings"
)

// netDebugSnapshot is a raw, best-effort dump of the router's kernel routing/
// firewall state. It exists purely to answer one question during TUN-mode
// debugging: does the kernel's forwarding path for LAN-client (non-router)
// traffic actually route through the TUN device, or does mihomo's
// "auto-route" only rewrite the router's own locally-originated default
// route? Each field is captured independently and never fails the whole
// response — a missing command just yields an empty string plus an error
// note, since this hits a live router and partial data still tells a story.
type netDebugSnapshot struct {
	IPForward       string   `json:"ip_forward"`
	RPFilterAll     string   `json:"rp_filter_all"`
	RPFilterDefault string   `json:"rp_filter_default"`
	RoutesMain      string   `json:"routes_main"`
	RoutesAllTables string   `json:"routes_all_tables"`
	RoutesTbl2022   string   `json:"routes_table_2022"`
	IPRules         string   `json:"ip_rules"`
	TunLinks        string   `json:"tun_links"`
	NftRuleset      string   `json:"nft_ruleset"`
	Conntrack       string   `json:"conntrack"`
	Errors          []string `json:"errors,omitempty"`
}

func runDebugCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func captureNetDebugSnapshot() netDebugSnapshot {
	var snap netDebugSnapshot
	note := func(label string, err error) {
		if err != nil {
			snap.Errors = append(snap.Errors, label+": "+err.Error())
		}
	}

	var err error
	snap.IPForward, err = runDebugCmd("cat", "/proc/sys/net/ipv4/ip_forward")
	note("ip_forward", err)

	snap.RPFilterAll, err = runDebugCmd("cat", "/proc/sys/net/ipv4/conf/all/rp_filter")
	note("rp_filter_all", err)

	snap.RPFilterDefault, err = runDebugCmd("cat", "/proc/sys/net/ipv4/conf/default/rp_filter")
	note("rp_filter_default", err)

	snap.RoutesMain, err = runDebugCmd("ip", "route", "show", "table", "main")
	note("routes_main", err)

	snap.RoutesAllTables, err = runDebugCmd("ip", "route", "show", "table", "all")
	note("routes_all_tables", err)

	snap.RoutesTbl2022, err = runDebugCmd("ip", "route", "show", "table", "2022")
	note("routes_table_2022", err)

	snap.IPRules, err = runDebugCmd("ip", "rule", "show")
	note("ip_rules", err)

	snap.TunLinks, err = runDebugCmd("sh", "-c", "ip -d link show type tun; ip -d link show type tap")
	note("tun_links", err)

	snap.NftRuleset, err = runDebugCmd("nft", "list", "ruleset")
	note("nft_ruleset", err)

	// conntrack -L falls back to /proc/net/nf_conntrack on minimal OpenWrt
	// images that don't ship the conntrack CLI. This is read once on demand
	// (not during the probe window), so it only shows whatever connection
	// state is still tracked at the moment netdiag's TUN-diagnostics section
	// runs — useful to see if TCP SYNs from a LAN client were tracked at all
	// (e.g. lingering in SYN_SENT) even when mihomo never logged a dispatch.
	snap.Conntrack, err = runDebugCmd("sh", "-c", "conntrack -L 2>/dev/null || cat /proc/net/nf_conntrack 2>/dev/null")
	note("conntrack", err)

	return snap
}

// handleNetDebug exposes a raw, read-only kernel routing/firewall snapshot so
// remote diagnostics (netdiag) can inspect whether TUN mode's auto-route is
// actually capturing forwarded LAN-client traffic, without needing SSH access.
func handleNetDebug(_ Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, captureNetDebugSnapshot())
	}
}
