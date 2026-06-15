package config

import (
	"strconv"
	"strings"
)

type MetaclashConfig struct {
	Core     CoreConfig     `toml:"core" json:"core"`
	Ports    PortsConfig    `toml:"ports" json:"ports"`
	Network  NetworkConfig  `toml:"network" json:"network"`
	DNS      DNSConfig      `toml:"dns" json:"dns"`
	Update   UpdateConfig   `toml:"update" json:"update"`
	Security SecurityConfig `toml:"security" json:"security"`
	Log      LogConfig      `toml:"log" json:"log"`
}

type CoreConfig struct {
	Binary        string `toml:"binary" json:"binary"`
	RuntimeDir    string `toml:"runtime_dir" json:"runtime_dir"`
	DataDir       string `toml:"data_dir" json:"data_dir"`
	GeoIPPath     string `toml:"geoip_path" json:"geoip_path"`
	GeositePath   string `toml:"geosite_path" json:"geosite_path"`
	MaxRestarts   int    `toml:"max_restarts" json:"max_restarts"`
	AutoStartCore bool   `toml:"auto_start_core" json:"auto_start_core"`
	// GeoDataMode controls mihomo's geodata-mode: false=mmdb format, true=dat format
	GeoDataMode bool `toml:"geodata_mode" json:"geodata_mode"`
}

type PortsConfig struct {
	HTTP      int `toml:"http" json:"http"`
	SOCKS     int `toml:"socks" json:"socks"`
	Mixed     int `toml:"mixed" json:"mixed"`
	Redir     int `toml:"redir" json:"redir"`
	TProxy    int `toml:"tproxy" json:"tproxy"`
	DNS       int `toml:"dns" json:"dns"`
	MihomoAPI int `toml:"mihomo_api" json:"mihomo_api"`
	UI        int `toml:"ui" json:"ui"`
}

// TUNConfig holds settings for mihomo's TUN virtual-NIC mode.
// Only active when NetworkConfig.Mode == "tun".
type TUNConfig struct {
	// Stack selects the TCP/IP stack: "system" | "gvisor" | "mixed" (default).
	// "mixed" uses gvisor for UDP and system for TCP — best compatibility on OpenWrt.
	Stack string `toml:"stack" json:"stack"`
	// DNSHijack is the list of DNS endpoints mihomo intercepts when TUN is active.
	// "any:53" intercepts all DNS traffic regardless of destination.
	DNSHijack []string `toml:"dns_hijack" json:"dns_hijack"`
	// AutoRoute lets mihomo install default routes pointing traffic into the TUN device.
	AutoRoute bool `toml:"auto_route" json:"auto_route"`
	// AutoDetectInterface lets mihomo pick the default WAN interface automatically.
	AutoDetectInterface bool `toml:"auto_detect_interface" json:"auto_detect_interface"`
	// Device is the TUN interface name (e.g. "Meta"). Leave empty for mihomo's default.
	Device string `toml:"device" json:"device"`
}

type NetworkConfig struct {
	// Mode controls how ClashForge intercepts traffic.
	// Valid values: "tproxy" | "redir" | "tun" | "none"
	Mode            string   `toml:"mode" json:"mode"`
	FirewallBackend string   `toml:"firewall_backend" json:"firewall_backend"`
	ApplyOnStart    bool     `toml:"apply_on_start" json:"apply_on_start"`
	BypassLAN       bool     `toml:"bypass_lan" json:"bypass_lan"`
	BypassChina     bool     `toml:"bypass_china" json:"bypass_china"`
	IPv6            bool     `toml:"ipv6" json:"ipv6"`
	BypassCIDR      []string `toml:"bypass_cidr" json:"bypass_cidr"`
	// WANInterface is the router's WAN-facing network interface used for the
	// dhcp:// nameserver entry so Mihomo reads ISP DNS from the DHCP lease.
	WANInterface string `toml:"wan_interface" json:"wan_interface"`
	// WANInterfaceAutoDetected is set to true when ClashForge corrected
	// WANInterface at startup because the configured value did not exist on
	// this system. Not persisted to TOML; used only for UI indication.
	WANInterfaceAutoDetected bool `toml:"-" json:"wan_interface_auto_detected"`
	// DropQUIC controls whether QUIC (UDP 443) is dropped at the nftables level,
	// forcing browsers to immediately fall back to TCP (which HTTP proxy nodes
	// can tunnel). Set to true when using HTTP proxy nodes that cannot tunnel UDP.
	// When false, QUIC bypasses tproxy entirely (legacy behaviour).
	DropQUIC bool `toml:"drop_quic" json:"drop_quic"`
	// TUN holds TUN-mode specific settings. Only used when Mode == "tun".
	TUN TUNConfig `toml:"tun" json:"tun"`
}

// DNSStrategy controls how Mihomo's nameserver-policy is generated.
//
//   - ""       / "legacy"  — backward-compatible: no nameserver-policy, rely on
//     fallback-filter GeoIP check after ISP DNS responds.
//   - "split"              — query-time routing: geosite:cn → bootstrap IPs (best
//     CN CDN), geosite:geolocation-!cn → international DoH, unknowns fall through
//     to fallback-filter as a safety net. Requires geosite.dat.
//   - "privacy"            — same routing as split, but nameserver is also replaced
//     with CN DoH so the ISP never sees any DNS query. Requires geosite.dat.
type DNSStrategy = string

const (
	DNSStrategyLegacy  DNSStrategy = "legacy"
	DNSStrategysplit   DNSStrategy = "split"
	DNSStrategyPrivacy DNSStrategy = "privacy"
)

type DNSConfig struct {
	Enable       bool     `toml:"enable" json:"enable"`
	Mode         string   `toml:"mode" json:"mode"`
	IPv6         bool     `toml:"ipv6" json:"ipv6"`
	Nameservers  []string `toml:"nameservers" json:"nameservers"`
	Fallback     []string `toml:"fallback" json:"fallback"`
	DoH          []string `toml:"doh" json:"doh"`
	FakeIPFilter []string `toml:"fake_ip_filter" json:"fake_ip_filter"`
	DnsmasqMode  string   `toml:"dnsmasq_mode" json:"dnsmasq_mode"`
	ApplyOnStart bool     `toml:"apply_on_start" json:"apply_on_start"`
	// Strategy selects the DNS routing mode. See DNSStrategy constants.
	// Empty string is treated as "legacy" for backward compatibility.
	Strategy DNSStrategy `toml:"strategy" json:"strategy"`
}

type UpdateConfig struct {
	AutoSubscription     bool   `toml:"auto_subscription" json:"auto_subscription"`
	SubscriptionInterval string `toml:"subscription_interval" json:"subscription_interval"`
	AutoGeoIP            bool   `toml:"auto_geoip" json:"auto_geoip"`
	GeoIPInterval        string `toml:"geoip_interval" json:"geoip_interval"`
	AutoGeosite          bool   `toml:"auto_geosite" json:"auto_geosite"`
	GeositeInterval      string `toml:"geosite_interval" json:"geosite_interval"`
	GeoIPURL             string `toml:"geoip_url" json:"geoip_url"`
	GeositeURL           string `toml:"geosite_url" json:"geosite_url"`
	// GeoDataProxyServer is the mihomo proxy name used when downloading GeoData files.
	// Empty or "DIRECT" means download without proxy. Any other value routes downloads
	// through mihomo's HTTP proxy port using its current active routing.
	GeoDataProxyServer string `toml:"geodata_proxy_server" json:"geodata_proxy_server"`

	// Self-update: automatically upgrade the clashforge package at a scheduled time.
	// AutoSelfUpdate enables the feature. SelfUpdateTime is "HH:MM" in local time
	// (default "02:00"). SelfUpdateChannel is "stable" or "preview" (default "stable").
	AutoSelfUpdate    bool   `toml:"auto_self_update" json:"auto_self_update"`
	SelfUpdateTime   string `toml:"self_update_time" json:"self_update_time"`
	SelfUpdateChannel string `toml:"self_update_channel" json:"self_update_channel"`
}

type SecurityConfig struct {
	APISecret string `toml:"api_secret" json:"api_secret"`
	AllowLAN  bool   `toml:"allow_lan" json:"allow_lan"`
}

type LogConfig struct {
	Level     string `toml:"level" json:"level"`
	File      string `toml:"file" json:"file"`
	MaxSizeMB int    `toml:"max_size_mb" json:"max_size_mb"`
}

func Default() *MetaclashConfig {
	return &MetaclashConfig{
		Core: CoreConfig{
			Binary:      "/usr/bin/mihomo",
			RuntimeDir:  "/var/run/metaclash",
			DataDir:     "/etc/metaclash",
			GeoIPPath:   "/usr/share/metaclash/Country.mmdb",
			GeositePath: "/usr/share/metaclash/geosite.dat",
			MaxRestarts: 3,
		},
		Ports:    PortsConfig{HTTP: 17890, SOCKS: 17891, Mixed: 17893, Redir: 17892, TProxy: 17895, DNS: 17874, MihomoAPI: 19090, UI: 7777},
		Network: NetworkConfig{
			Mode: "tproxy", FirewallBackend: "auto", ApplyOnStart: true,
			BypassLAN: true, BypassChina: true, IPv6: false, BypassCIDR: []string{},
			WANInterface: "eth1", DropQUIC: true,
			TUN: TUNConfig{
				Stack:               "mixed",
				DNSHijack:           []string{"any:53"},
				AutoRoute:           true,
				AutoDetectInterface: true,
			},
		},
		DNS:      DNSConfig{Enable: true, Mode: "fake-ip", Nameservers: []string{"223.5.5.5", "119.29.29.29"}, Fallback: []string{"tls://8.8.4.4", "tls://1.1.1.1", "https://dns.google/dns-query", "https://cloudflare-dns.com/dns-query"}, DoH: []string{}, FakeIPFilter: []string{"+.lan", "+.local", "time.*.com", "ntp.*.com", "+.ntp.org", "+.qq.com", "+.qpic.cn", "+.qlogo.cn", "+.myqcloud.com", "+.qcloud.com", "+.tencent.com", "+.wechat.com", "+.weixin.com", "+.tencentcs.com", "+.gtimg.com", "+.weiyun.com", "+.taobao.com", "+.tmall.com", "+.alipay.com", "+.aliyun.com", "+.alibaba.com", "+.alicdn.com", "+.baidu.com", "+.bdstatic.com", "+.bytedance.com", "+.douyin.com", "+.ixigua.com"}, DnsmasqMode: "none", ApplyOnStart: true, Strategy: DNSStrategysplit},
		Update:   UpdateConfig{AutoSubscription: true, SubscriptionInterval: "6h", AutoGeoIP: true, GeoIPInterval: "168h", AutoGeosite: true, GeositeInterval: "168h", GeoIPURL: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb", GeositeURL: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat", AutoSelfUpdate: false, SelfUpdateTime: "02:00", SelfUpdateChannel: "stable"},
		Security: SecurityConfig{APISecret: "", AllowLAN: true},
		Log:      LogConfig{Level: "info", File: "", MaxSizeMB: 10},
	}
}

func (c *MetaclashConfig) Redacted() *MetaclashConfig {
	cp := *c
	cp.Security = c.Security
	if strings.TrimSpace(cp.Security.APISecret) != "" {
		cp.Security.APISecret = "***"
	}
	return &cp
}

func (c *MetaclashConfig) UIListenAddr() string {
	if c.Security.AllowLAN {
		return ":" + itoa(c.Ports.UI)
	}
	return "127.0.0.1:" + itoa(c.Ports.UI)
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
