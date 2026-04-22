package subscription

import (
	"fmt"
	"net/url"
	"strconv"
)

func parseVless(link string) (*ProxyNode, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		return nil, fmt.Errorf("vless missing port")
	}
	name, _ := url.PathUnescape(u.Fragment)
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	node := &ProxyNode{
		Name:   name,
		Type:   "vless",
		Server: u.Hostname(),
		Port:   port,
		Extra:  map[string]interface{}{},
	}
	node.Extra["uuid"] = u.User.Username()

	q := u.Query()
	if flow := q.Get("flow"); flow != "" {
		node.Extra["flow"] = flow
	}
	if sni := q.Get("sni"); sni != "" {
		node.Extra["servername"] = sni
	}
	if q.Get("security") == "tls" || q.Get("security") == "reality" {
		node.Extra["tls"] = true
	}
	if fp := q.Get("fp"); fp != "" {
		node.Extra["client-fingerprint"] = fp
	}
	if pbk := q.Get("pbk"); pbk != "" {
		node.Extra["reality-opts"] = map[string]interface{}{
			"public-key": pbk,
			"short-id":   q.Get("sid"),
		}
	}
	switch q.Get("type") {
	case "ws":
		node.Extra["network"] = "ws"
		opts := map[string]interface{}{}
		if h := q.Get("host"); h != "" {
			opts["headers"] = map[string]string{"Host": h}
		}
		if p := q.Get("path"); p != "" {
			opts["path"] = p
		}
		node.Extra["ws-opts"] = opts
	case "grpc":
		node.Extra["network"] = "grpc"
		if sn := q.Get("serviceName"); sn != "" {
			node.Extra["grpc-opts"] = map[string]string{"grpc-service-name": sn}
		}
	case "h2":
		node.Extra["network"] = "h2"
	}
	return node, nil
}
