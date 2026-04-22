package subscription

import (
	"bytes"
	"encoding/base64"
	"strings"
)

// Parse detects format and returns ProxyNode list.
func Parse(content []byte) ([]ProxyNode, error) {
	trimmed := bytes.TrimSpace(content)

	// 1. Try Clash YAML
	if bytes.Contains(trimmed, []byte("proxies:")) {
		nodes, err := parseClashYAML(trimmed)
		if err == nil && len(nodes) > 0 {
			return nodes, nil
		}
	}

	// 2. Try base64 decode
	if looksLikeBase64(trimmed) {
		decoded, err := base64DecodeAny(string(trimmed))
		if err == nil {
			return parseLineBased(decoded)
		}
	}

	// 3. Line-based
	return parseLineBased(trimmed)
}

func parseLineBased(content []byte) ([]ProxyNode, error) {
	var nodes []ProxyNode
	for _, rawLine := range bytes.Split(content, []byte("\n")) {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var node *ProxyNode
		var err error
		switch {
		case strings.HasPrefix(line, "vmess://"):
			node, err = parseVmess(line)
		case strings.HasPrefix(line, "trojan://"):
			node, err = parseTrojan(line)
		case strings.HasPrefix(line, "ss://"):
			node, err = parseSS(line)
		case strings.HasPrefix(line, "vless://"):
			node, err = parseVless(line)
		default:
			continue
		}
		if err != nil || node == nil {
			continue
		}
		nodes = append(nodes, *node)
	}
	return nodes, nil
}

func looksLikeBase64(data []byte) bool {
	s := strings.TrimSpace(string(data))
	if len(s) < 8 {
		return false
	}
	// Should not contain colons or slashes that indicate URLs
	if strings.Contains(s, "://") || strings.Contains(s, " ") {
		return false
	}
	validChars := 0
	for _, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' || c == '-' || c == '_' {
			validChars++
		}
	}
	return float64(validChars)/float64(len(s)) > 0.95
}

func base64DecodeAny(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	// Try standard first, then URL-safe
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}
	// Pad if needed
	for pad := 0; pad < 4; pad++ {
		padded := s + strings.Repeat("=", pad)
		for _, enc := range encodings {
			if data, err := enc.DecodeString(padded); err == nil {
				return data, nil
			}
		}
	}
	return nil, bytes.ErrTooLarge
}
