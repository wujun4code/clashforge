package subscription

import (
	"fmt"
	"net/url"
	"strconv"
)

func parseTrojan(link string) (*ProxyNode, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		return nil, fmt.Errorf("trojan missing port")
	}
	name, _ := url.PathUnescape(u.Fragment)
	if name == "" {
		name = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	node := &ProxyNode{
		Name:   name,
		Type:   "trojan",
		Server: u.Hostname(),
		Port:   port,
		Extra:  map[string]interface{}{},
	}
	node.Extra["password"] = u.User.Username()

	q := u.Query()
	if sni := q.Get("sni"); sni != "" {
		node.Extra["sni"] = sni
	}
	if q.Get("allowInsecure") == "1" {
		node.Extra["skip-cert-verify"] = true
	}
	if fp := q.Get("fp"); fp != "" {
		node.Extra["client-fingerprint"] = fp
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
	}
	return node, nil
}
