package subscription

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// stripCommonIndent removes a uniform leading indent shared by every non-blank,
// non-comment line. This handles the case where the user copies a proxy list
// from inside a "proxies:" YAML block — every line (including each "- ") has
// the same leading spaces that need to be removed before the bare-sequence
// parser can treat it as a top-level YAML sequence.
func stripCommonIndent(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	min := -1
	for _, line := range lines {
		stripped := bytes.TrimLeft(line, " ")
		if len(stripped) == 0 || bytes.HasPrefix(stripped, []byte("#")) {
			continue
		}
		n := len(line) - len(stripped)
		if min < 0 || n < min {
			min = n
		}
	}
	if min <= 0 {
		return content
	}
	out := make([][]byte, len(lines))
	for i, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			out[i] = line
			continue
		}
		removed := 0
		for removed < min && removed < len(line) && line[removed] == ' ' {
			removed++
		}
		out[i] = line[removed:]
	}
	return bytes.Join(out, []byte("\n"))
}

// normalizeBareSeqIndent fixes a common copy-paste artefact: when a proxy list
// is copied from inside a "proxies:" block, the dash "-" ends up at col 0 but
// continuation keys remain at col 4 (where they were under "  - ").  That is
// invalid YAML because the first key ("name", inline with "- ") is at col 2.
// This function strips the excess indentation from continuation lines so that
// every key within a sequence item aligns at col 2.
// Relative indentation within nested sub-blocks is preserved.
func normalizeBareSeqIndent(content []byte) []byte {
	lines := bytes.Split(content, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	excess := 0

	for _, raw := range lines {
		stripped := bytes.TrimLeft(raw, " ")
		if len(stripped) == 0 {
			out = append(out, raw)
			continue
		}
		// New sequence item — reset excess for this item.
		if bytes.HasPrefix(stripped, []byte("- ")) || bytes.Equal(bytes.TrimRight(stripped, "\r"), []byte("-")) {
			excess = 0
			out = append(out, raw)
			continue
		}
		// Continuation line inside a sequence item.
		indent := len(raw) - len(stripped)
		if excess == 0 && indent > 2 {
			excess = indent - 2
		}
		if excess > 0 && indent >= excess {
			line := make([]byte, 0, len(raw)-excess)
			for i := 0; i < indent-excess; i++ {
				line = append(line, ' ')
			}
			line = append(line, stripped...)
			out = append(out, line)
		} else {
			out = append(out, raw)
		}
	}
	return bytes.Join(out, []byte("\n"))
}

func parseClashYAML(content []byte) ([]ProxyNode, error) {
	// Try full Clash config with proxies: key
	var raw struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(content, &raw); err == nil && len(raw.Proxies) > 0 {
		return extractProxyNodes(raw.Proxies), nil
	}
	// Try bare YAML sequence (list without proxies: wrapper).
	// Normalise continuation-line excess indent.
	normalized := normalizeBareSeqIndent(content)
	var rawList []map[string]interface{}
	if err := yaml.Unmarshal(normalized, &rawList); err != nil {
		return nil, fmt.Errorf("clash yaml parse: %w", err)
	}
	if len(rawList) == 0 {
		return nil, fmt.Errorf("clash yaml parse: no proxy entries found")
	}
	return extractProxyNodes(rawList), nil
}

func extractProxyNodes(proxies []map[string]interface{}) []ProxyNode {
	var nodes []ProxyNode
	for _, p := range proxies {
		name, _ := p["name"].(string)
		typ, _ := p["type"].(string)
		server, _ := p["server"].(string)
		port := toIntVal(p["port"], 0)
		if name == "" || typ == "" || server == "" || port == 0 {
			continue
		}
		extra := make(map[string]interface{})
		for k, v := range p {
			if k != "name" && k != "type" && k != "server" && k != "port" {
				extra[k] = v
			}
		}
		nodes = append(nodes, ProxyNode{
			Name:   name,
			Type:   typ,
			Server: server,
			Port:   port,
			Extra:  extra,
		})
	}
	return nodes
}
