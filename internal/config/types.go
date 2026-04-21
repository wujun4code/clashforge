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
	Binary      string `toml:"binary" json:"binary"`
	RuntimeDir  string `toml:"runtime_dir" json:"runtime_dir"`
	DataDir     string `toml:"data_dir" json:"data_dir"`
	GeoIPPath   string `toml:"geoip_path" json:"geoip_path"`
	GeositePath string `toml:"geosite_path" json:"geosite_path"`
	MaxRestarts int    `toml:"max_restarts" json:"max_restarts"`
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

type NetworkConfig struct {
	Mode            string   `toml:"mode" json:"mode"`
	FirewallBackend string   `toml:"firewall_backend" json:"firewall_backend"`
	BypassLAN       bool     `toml:"bypass_lan" json:"bypass_lan"`
	BypassChina     bool     `toml:"bypass_china" json:"bypass_china"`
	IPv6            bool     `toml:"ipv6" json:"ipv6"`
	BypassCIDR      []string `toml:"bypass_cidr" json:"bypass_cidr"`
}

type DNSConfig struct {
	Enable       bool     `toml:"enable" json:"enable"`
	Mode         string   `toml:"mode" json:"mode"`
	Nameservers  []string `toml:"nameservers" json:"nameservers"`
	Fallback     []string `toml:"fallback" json:"fallback"`
	DoH          []string `toml:"doh" json:"doh"`
	FakeIPFilter []string `toml:"fake_ip_filter" json:"fake_ip_filter"`
	DnsmasqMode  string   `toml:"dnsmasq_mode" json:"dnsmasq_mode"`
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
		Ports:    PortsConfig{HTTP: 7890, SOCKS: 7891, Mixed: 7893, Redir: 7892, TProxy: 7895, DNS: 7874, MihomoAPI: 9090, UI: 7777},
		Network:  NetworkConfig{Mode: "tproxy", FirewallBackend: "auto", BypassLAN: true, BypassChina: false, IPv6: false, BypassCIDR: []string{}},
		DNS:      DNSConfig{Enable: true, Mode: "fake-ip", Nameservers: []string{"119.29.29.29", "223.5.5.5"}, Fallback: []string{"8.8.8.8", "1.1.1.1"}, DoH: []string{"https://doh.pub/dns-query"}, FakeIPFilter: []string{"+.lan", "+.local", "time.*.com", "ntp.*.com", "+.ntp.org"}, DnsmasqMode: "upstream"},
		Update:   UpdateConfig{AutoSubscription: true, SubscriptionInterval: "6h", AutoGeoIP: true, GeoIPInterval: "168h", AutoGeosite: true, GeositeInterval: "168h", GeoIPURL: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb", GeositeURL: "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"},
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
