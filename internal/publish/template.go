package publish

import (
	"fmt"
	"strings"
)

const loyalSoldierTemplateYAML = `mode: rule
log-level: info
allow-lan: true
proxy-groups:
  - name: "🚀 节点选择"
    type: select
    proxies:
      - DIRECT
  - name: "🎯 全球直连"
    type: select
    proxies:
      - DIRECT
      - "🚀 节点选择"
  - name: "🐟 漏网之鱼"
    type: select
    proxies:
      - "🚀 节点选择"
      - "🎯 全球直连"
rule-providers:
  reject:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/reject.txt
    path: "./rule_provider/reject.yaml"
    interval: 86400
  icloud:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/icloud.txt
    path: "./rule_provider/icloud.yaml"
    interval: 86400
  apple:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/apple.txt
    path: "./rule_provider/apple.yaml"
    interval: 86400
  google:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/google.txt
    path: "./rule_provider/google.yaml"
    interval: 86400
  proxy:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/proxy.txt
    path: "./rule_provider/proxy.yaml"
    interval: 86400
  direct:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/direct.txt
    path: "./rule_provider/direct.yaml"
    interval: 86400
  private:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/private.txt
    path: "./rule_provider/private.yaml"
    interval: 86400
  gfw:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/gfw.txt
    path: "./rule_provider/gfw.yaml"
    interval: 86400
  tld-not-cn:
    type: http
    behavior: domain
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/tld-not-cn.txt
    path: "./rule_provider/tld-not-cn.yaml"
    interval: 86400
  telegramcidr:
    type: http
    behavior: ipcidr
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/telegramcidr.txt
    path: "./rule_provider/telegramcidr.yaml"
    interval: 86400
  cncidr:
    type: http
    behavior: ipcidr
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/cncidr.txt
    path: "./rule_provider/cncidr.yaml"
    interval: 86400
  lancidr:
    type: http
    behavior: ipcidr
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/lancidr.txt
    path: "./rule_provider/lancidr.yaml"
    interval: 86400
  applications:
    type: http
    behavior: classical
    url: https://cdn.jsdmirror.com/gh/Loyalsoldier/clash-rules@release/applications.txt
    path: "./rule_provider/applications.yaml"
    interval: 86400
rules:
  - RULE-SET,reject,REJECT
  - RULE-SET,private,🎯 全球直连
  - RULE-SET,applications,🎯 全球直连
  - RULE-SET,icloud,🎯 全球直连
  - RULE-SET,apple,🎯 全球直连
  - RULE-SET,lancidr,🎯 全球直连,no-resolve
  - GEOIP,LAN,🎯 全球直连
  - RULE-SET,google,🚀 节点选择
  - RULE-SET,gfw,🚀 节点选择
  - RULE-SET,proxy,🚀 节点选择
  - RULE-SET,direct,🎯 全球直连
  - RULE-SET,tld-not-cn,🚀 节点选择
  - RULE-SET,telegramcidr,🚀 节点选择,no-resolve
  - RULE-SET,cncidr,🎯 全球直连,no-resolve
  - GEOIP,CN,🎯 全球直连
  - MATCH,🐟 漏网之鱼
`

const minimalTemplateYAML = `mode: rule
log-level: info
proxy-groups:
  - name: "🚀 节点选择"
    type: select
    proxies:
      - DIRECT
rules:
  - GEOIP,CN,DIRECT
  - MATCH,🚀 节点选择
`

func ListTemplatePresets() []TemplatePreset {
	return []TemplatePreset{
		{
			ID:          "loyalsoldier_standard",
			Name:        "Loyalsoldier 标准规则",
			Description: "内置常用 rule-providers 与规则顺序，适合作为通用科学上网模板。",
		},
		{
			ID:          "minimal",
			Name:        "极简模板",
			Description: "仅包含基础分流规则，便于快速验证节点与订阅链路。",
		},
	}
}

func TemplateByID(id string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(id)) {
	case "", "loyalsoldier_standard", "loyalsoldier", "standard":
		return loyalSoldierTemplateYAML, nil
	case "minimal":
		return minimalTemplateYAML, nil
	default:
		return "", fmt.Errorf("unknown template id: %s", id)
	}
}

func ResolveTemplate(mode, templateID, templateContent, runtimeContent string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", "builtin":
		return TemplateByID(templateID)
	case "runtime":
		raw := strings.TrimSpace(runtimeContent)
		if raw == "" {
			return "", fmt.Errorf("runtime template is empty")
		}
		return raw, nil
	case "custom":
		raw := strings.TrimSpace(templateContent)
		if raw == "" {
			return "", fmt.Errorf("custom template is empty")
		}
		return raw, nil
	default:
		return "", fmt.Errorf("unsupported template_mode: %s", mode)
	}
}
