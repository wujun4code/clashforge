package subscription

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type vmessJSON struct {
	V    string      `json:"v"`
	PS   string      `json:"ps"`
	Add  string      `json:"add"`
	Port interface{} `json:"port"`
	ID   string      `json:"id"`
	Aid  interface{} `json:"aid"`
	Scy  string      `json:"scy"`
	Net  string      `json:"net"`
	Type string      `json:"type"`
	Host string      `json:"host"`
	Path string      `json:"path"`
	TLS  string      `json:"tls"`
	SNI  string      `json:"sni"`
	ALPN string      `json:"alpn"`
	FP   string      `json:"fp"`
}

func parseVmess(link string) (*ProxyNode, error) {
	b64 := strings.TrimPrefix(link, "vmess://")
	data, err := base64DecodeAny(b64)
	if err != nil {
		return nil, fmt.Errorf("vmess base64: %w", err)
	}
	var v vmessJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("vmess json: %w", err)
	}
	port, err := parsePortAny(v.Port)
	if err != nil || port == 0 {
		return nil, fmt.Errorf("vmess invalid port: %v", v.Port)
	}
	name := v.PS
	if name == "" {
		name = fmt.Sprintf("%s:%d", v.Add, port)
	}
	node := &ProxyNode{
		Name:   name,
		Type:   "vmess",
		Server: v.Add,
		Port:   port,
		Extra:  map[string]interface{}{},
	}
	node.Extra["uuid"] = v.ID
	node.Extra["alterId"] = toIntVal(v.Aid, 0)
	if v.Scy != "" {
		node.Extra["cipher"] = v.Scy
	} else {
		node.Extra["cipher"] = "auto"
	}
	if v.Net != "" && v.Net != "tcp" {
		node.Extra["network"] = v.Net
		opts := map[string]interface{}{}
		switch v.Net {
		case "ws":
			if v.Host != "" {
				opts["headers"] = map[string]string{"Host": v.Host}
			}
			if v.Path != "" {
				opts["path"] = v.Path
			}
			node.Extra["ws-opts"] = opts
		case "grpc":
			if v.Path != "" {
				opts["grpc-service-name"] = v.Path
			}
			node.Extra["grpc-opts"] = opts
		case "h2":
			if v.Host != "" {
				opts["host"] = []string{v.Host}
			}
			if v.Path != "" {
				opts["path"] = v.Path
			}
			node.Extra["h2-opts"] = opts
		}
	}
	if v.TLS == "tls" || v.TLS == "reality" {
		node.Extra["tls"] = true
		if v.SNI != "" {
			node.Extra["servername"] = v.SNI
		}
		if v.FP != "" {
			node.Extra["client-fingerprint"] = v.FP
		}
	}
	return node, nil
}

func parsePortAny(v interface{}) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case string:
		p, err := strconv.Atoi(val)
		return p, err
	default:
		return 0, fmt.Errorf("unknown port type %T", v)
	}
}

func toIntVal(v interface{}, def int) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		if p, err := strconv.Atoi(val); err == nil {
			return p
		}
	}
	return def
}
