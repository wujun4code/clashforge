package probetargets

// HTTPTarget defines a named HTTP probe target.
type HTTPTarget struct {
	Name        string
	Group       string
	URL         string
	Description string
}

// IPProviderTarget defines a provider used for exit-IP probe.
type IPProviderTarget struct {
	Name  string
	Group string
	URL   string
	GBK   bool
}

// ConnectivityTargets are the canonical targets used by router/browser
// connectivity probes in Dashboard/Setup.
func ConnectivityTargets() []HTTPTarget {
	return []HTTPTarget{
		{Name: "淘宝", Group: "国内", URL: "https://www.taobao.com", Description: "用于验证国内主要电商平台的直连可达性。"},
		{Name: "网易云音乐", Group: "国内", URL: "https://music.163.com", Description: "用于验证国内常见内容站点延迟。"},
		{Name: "GitHub", Group: "国外", URL: "https://github.com", Description: "用于验证国际开发站点的代理访问效果。"},
		{Name: "Google", Group: "国外", URL: "https://www.google.com", Description: "用于验证 Google 搜索是否可通过代理访问。"},
		{Name: "OpenAI", Group: "AI", URL: "https://chat.openai.com", Description: "用于验证 ChatGPT / OpenAI 是否可通过代理访问。"},
		{Name: "Gemini", Group: "AI", URL: "https://gemini.google.com", Description: "用于验证 Google Gemini 是否可通过代理访问。"},
	}
}

// IPProviderTargets are the canonical providers used by exit-IP probes.
func IPProviderTargets() []IPProviderTarget {
	return []IPProviderTarget{
		{Name: "太平洋", Group: "国内", URL: "https://whois.pconline.com.cn/ipJson.jsp?json=true", GBK: true},
		{Name: "UpaiYun", Group: "国内", URL: "https://pubstatic.b0.upaiyun.com/?_upnode"},
		{Name: "IP.SB", Group: "国外", URL: "https://api.ip.sb/geoip"},
		{Name: "IPInfo", Group: "国外", URL: "https://ipinfo.io/json"},
	}
}

// NodeConnectivityTargets are used by node deployment/probe flow.
func NodeConnectivityTargets() []HTTPTarget {
	return []HTTPTarget{
		{Name: "Google", Group: "国外", URL: "https://www.google.com"},
		{Name: "YouTube", Group: "国外", URL: "https://www.youtube.com"},
		{Name: "GitHub", Group: "国外", URL: "https://github.com"},
	}
}

// DefaultHealthCheckTargetURL is the default URL for /health/check when target is empty.
func DefaultHealthCheckTargetURL() string {
	targets := ConnectivityTargets()
	for _, target := range targets {
		if target.Name == "Google" {
			return target.URL
		}
	}
	if len(targets) > 0 {
		return targets[0].URL
	}
	return "https://www.google.com"
}
