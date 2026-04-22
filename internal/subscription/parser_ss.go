package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func parseSS(link string) (*ProxyNode, error) {
	withoutScheme := strings.TrimPrefix(link, "ss://")
	if strings.Contains(withoutScheme, "@") {
		return parseSSIP002(link)
	}
	return parseSSLegacy(link)
}

// SIP002: ss://BASE64(method:password)@server:port[#name]
// or ss://method:password@server:port[#name]
func parseSSIP002(link string) (*ProxyNode, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		return nil, fmt.Errorf("ss missing port")
	}
	name, _ := url.PathUnescape(u.Fragment)
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}

	var method, password string
	userInfo := u.User.Username()
	// Try base64 decode first
	decoded, err := base64DecodeAny(userInfo)
	if err == nil && strings.Contains(string(decoded), ":") {
		parts := strings.SplitN(string(decoded), ":", 2)
		method, password = parts[0], parts[1]
	} else if strings.Contains(userInfo, ":") {
		parts := strings.SplitN(userInfo, ":", 2)
		method = parts[0]
		password = parts[1]
	} else {
		method = userInfo
		if pw, ok := u.User.Password(); ok {
			password = pw
		}
	}

	return &ProxyNode{
		Name:   name,
		Type:   "ss",
		Server: u.Hostname(),
		Port:   port,
		Extra: map[string]interface{}{
			"cipher":   method,
			"password": password,
		},
	}, nil
}

// Legacy: ss://BASE64(method:password@server:port)[#name]
func parseSSLegacy(link string) (*ProxyNode, error) {
	withoutScheme := strings.TrimPrefix(link, "ss://")
	name := ""
	if idx := strings.Index(withoutScheme, "#"); idx != -1 {
		name, _ = url.PathUnescape(withoutScheme[idx+1:])
		withoutScheme = withoutScheme[:idx]
	}
	decoded, err := base64DecodeAny(withoutScheme)
	if err != nil {
		return nil, fmt.Errorf("ss legacy base64: %w", err)
	}
	// format: method:password@server:port
	u, err := url.Parse("ss://" + string(decoded))
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(u.Port())
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	password, _ := u.User.Password()
	return &ProxyNode{
		Name:   name,
		Type:   "ss",
		Server: u.Hostname(),
		Port:   port,
		Extra: map[string]interface{}{
			"cipher":   u.User.Username(),
			"password": password,
		},
	}, nil
}
