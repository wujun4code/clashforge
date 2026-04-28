package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultIPv4Prefix             = 32
	defaultIPv6Prefix             = 128
	managedDeviceProviderPrefix   = "cf-device-group-"
	managedDeviceProviderDir      = "rule_provider"
	managedDeviceProviderPathTmpl = "./" + managedDeviceProviderDir + "/%s"
)

// DeviceGroup stores per-device routing preferences.
type DeviceGroup struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Devices   []Device             `json:"devices"`
	Overrides []ProxyGroupOverride `json:"overrides"`
	Order     int                  `json:"order"`
}

// Device identifies a source endpoint by CIDR.
type Device struct {
	IP       string `json:"ip"`
	Prefix   int    `json:"prefix"`
	Hostname string `json:"hostname,omitempty"`
}

// ProxyGroupOverride maps one original proxy group to a custom proxy subset.
type ProxyGroupOverride struct {
	OriginalGroup string   `json:"original_group"`
	Proxies       []string `json:"proxies"`
}

type deviceGroupFile struct {
	DeviceGroups []DeviceGroup `json:"device_groups"`
}

// DeviceRuleProviderSpec describes one generated device-group ipcidr rule-provider.
type DeviceRuleProviderSpec struct {
	Name     string
	FileName string
	Payload  []string
}

// DeviceGroupsPath returns the JSON path used to persist per-device rules.
func DeviceGroupsPath(dataDir string) string {
	return filepath.Join(dataDir, "device-groups.json")
}

// LoadDeviceGroups reads device groups from JSON.
// It supports both { "device_groups": [...] } and legacy plain array formats.
func LoadDeviceGroups(path string) ([]DeviceGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read device groups: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}

	var wrapped deviceGroupFile
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.DeviceGroups != nil {
		return normalizeDeviceGroups(wrapped.DeviceGroups), nil
	}

	var plain []DeviceGroup
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, fmt.Errorf("parse device groups: %w", err)
	}
	return normalizeDeviceGroups(plain), nil
}

// SaveDeviceGroups writes device groups to JSON atomically.
func SaveDeviceGroups(path string, groups []DeviceGroup) error {
	normalized := normalizeDeviceGroups(groups)
	payload := deviceGroupFile{DeviceGroups: normalized}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal device groups: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create device groups dir: %w", err)
	}

	f, err := os.CreateTemp(filepath.Dir(path), ".device-groups-*.json")
	if err != nil {
		return fmt.Errorf("create temp device groups: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write temp device groups: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp device groups: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace device groups: %w", err)
	}
	return nil
}

// SyncDeviceRuleProviderFiles writes generated device-group rule-provider files into
// runtimeDir/rule_provider and cleans up stale managed files.
func SyncDeviceRuleProviderFiles(runtimeDir string, specs []DeviceRuleProviderSpec) error {
	ruleProviderDir := filepath.Join(runtimeDir, managedDeviceProviderDir)
	if err := os.MkdirAll(ruleProviderDir, 0o755); err != nil {
		return fmt.Errorf("create device rule-provider dir: %w", err)
	}

	entries, err := os.ReadDir(ruleProviderDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read device rule-provider dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, managedDeviceProviderPrefix) || !strings.HasSuffix(strings.ToLower(name), ".yaml") {
			continue
		}
		if removeErr := os.Remove(filepath.Join(ruleProviderDir, name)); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove stale device rule-provider %s: %w", name, removeErr)
		}
	}

	for _, spec := range specs {
		fileName := strings.TrimSpace(spec.FileName)
		if fileName == "" {
			continue
		}
		content := marshalDeviceRuleProviderPayload(spec.Payload)
		if writeErr := os.WriteFile(filepath.Join(ruleProviderDir, fileName), content, 0o644); writeErr != nil {
			return fmt.Errorf("write device rule-provider %s: %w", fileName, writeErr)
		}
	}
	return nil
}

func marshalDeviceRuleProviderPayload(payload []string) []byte {
	builder := strings.Builder{}
	builder.WriteString("payload:\n")
	count := 0
	for _, cidr := range payload {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		builder.WriteString("  - ")
		builder.WriteString(cidr)
		builder.WriteByte('\n')
		count++
	}
	if count == 0 {
		return []byte("payload: []\n")
	}
	return []byte(builder.String())
}

// ApplyPerDeviceSubRules injects per-device shadow proxy-groups and AND rules
// according to device-group overrides.
func ApplyPerDeviceSubRules(base map[string]interface{}, groups []DeviceGroup) map[string]interface{} {
	result, _ := ApplyPerDeviceSubRulesWithProviders(base, groups)
	return result
}

// ApplyPerDeviceSubRulesWithProviders injects per-device shadow proxy-groups and rules,
// and returns generated ipcidr rule-provider specs that callers should persist.
func ApplyPerDeviceSubRulesWithProviders(base map[string]interface{}, groups []DeviceGroup) (map[string]interface{}, []DeviceRuleProviderSpec) {
	if base == nil || len(groups) == 0 {
		return base, nil
	}

	normalized := normalizeDeviceGroups(groups)
	if len(normalized) == 0 {
		return base, nil
	}

	proxyGroups, proxyGroupNames := readProxyGroups(base["proxy-groups"])
	if len(proxyGroups) == 0 {
		return base, nil
	}

	allowedProxies := collectAllowedProxyNames(base)
	validContexts := buildDeviceRuleContexts(normalized, proxyGroupNames, allowedProxies)
	if len(validContexts) == 0 {
		return base, nil
	}

	providerSpecs := collectDeviceRuleProviderSpecs(validContexts)
	upsertManagedDeviceRuleProviders(base, providerSpecs)

	for _, ctx := range validContexts {
		for originalGroup, shadow := range ctx.Shadows {
			origin, ok := proxyGroupNames[originalGroup]
			if !ok {
				continue
			}
			shadowGroup := cloneMap(origin)
			shadowGroup["name"] = shadow.Name
			shadowGroup["type"] = "select"
			shadowGroup["proxies"] = toInterfaceSlice(shadow.Proxies)
			upsertProxyGroup(&proxyGroups, shadowGroup)
		}
	}
	base["proxy-groups"] = proxyGroups

	rules := readRules(base["rules"])
	if len(rules) == 0 {
		return base, providerSpecs
	}

	overriddenGroups := map[string]bool{}
	for _, ctx := range validContexts {
		for original := range ctx.Shadows {
			overriddenGroups[original] = true
		}
	}
	if len(overriddenGroups) == 0 {
		return base, providerSpecs
	}

	andRulesByRuleIndex := make(map[int][]string)
	andRuleCount := 0
	matchDeviceRules := make([]string, 0)
	for i, rule := range rules {
		parsed, ok := parseRuleLine(rule)
		if !ok {
			continue
		}
		if !overriddenGroups[parsed.Policy] {
			continue
		}

		if strings.EqualFold(parsed.RuleType, "MATCH") {
			// Mihomo does not support MATCH inside logic rules like:
			// AND,((SRC-IP-CIDR,...),(MATCH)),...
			// Use an equivalent per-device fallback rule and keep global MATCH as final fallback.
			for _, ctx := range validContexts {
				shadow, ok := ctx.Shadows[parsed.Policy]
				if !ok {
					continue
				}
				matchDeviceRules = append(matchDeviceRules, fmt.Sprintf("RULE-SET,%s,%s,src,no-resolve", ctx.DeviceProvider.Name, shadow.Name))
			}
			continue
		}

		matcherParts := append([]string{}, parsed.Parts[:parsed.PolicyIndex]...)
		matcherParts = append(matcherParts, parsed.Parts[parsed.PolicyIndex+1:]...)
		matcher := strings.Join(matcherParts, ",")
		if strings.TrimSpace(matcher) == "" {
			continue
		}

		for _, ctx := range validContexts {
			shadow, ok := ctx.Shadows[parsed.Policy]
			if !ok {
				continue
			}
			andRulesByRuleIndex[i] = append(andRulesByRuleIndex[i], fmt.Sprintf("AND,((RULE-SET,%s,src,no-resolve),(%s)),%s", ctx.DeviceProvider.Name, matcher, shadow.Name))
			andRuleCount++
		}
	}
	if andRuleCount == 0 && len(matchDeviceRules) == 0 {
		return base, providerSpecs
	}

	expanded := make([]string, 0, len(rules)+andRuleCount)
	for i, rule := range rules {
		if inserts, ok := andRulesByRuleIndex[i]; ok && len(inserts) > 0 {
			expanded = append(expanded, inserts...)
		}
		expanded = append(expanded, rule)
	}
	if len(matchDeviceRules) > 0 {
		insertAt := firstMatchRuleIndex(expanded)
		withMatchFallback := make([]string, 0, len(expanded)+len(matchDeviceRules))
		withMatchFallback = append(withMatchFallback, expanded[:insertAt]...)
		withMatchFallback = append(withMatchFallback, matchDeviceRules...)
		withMatchFallback = append(withMatchFallback, expanded[insertAt:]...)
		expanded = withMatchFallback
	}

	base["rules"] = toInterfaceSlice(expanded)
	return base, providerSpecs
}

type shadowProxyGroup struct {
	Name    string
	Proxies []string
}

type deviceRuleContext struct {
	Shadows        map[string]shadowProxyGroup
	DeviceProvider DeviceRuleProviderSpec
}

func buildDeviceRuleContexts(groups []DeviceGroup, existingGroups map[string]map[string]interface{}, allowedProxies map[string]bool) []deviceRuleContext {
	contexts := make([]deviceRuleContext, 0)
	usedProviderNames := map[string]bool{}
	for _, g := range groups {
		groupName := strings.TrimSpace(g.Name)
		if groupName == "" {
			continue
		}

		payload := make([]string, 0, len(g.Devices))
		for _, d := range g.Devices {
			if cidr, ok := deviceCIDR(d); ok {
				payload = append(payload, cidr)
			}
		}
		payload = dedupeStrings(payload)
		if len(payload) == 0 {
			continue
		}

		shadows := map[string]shadowProxyGroup{}
		for _, ov := range g.Overrides {
			originalGroup := strings.TrimSpace(ov.OriginalGroup)
			if originalGroup == "" {
				continue
			}
			if _, ok := existingGroups[originalGroup]; !ok {
				continue
			}
			validProxies := filterProxyList(ov.Proxies, allowedProxies)
			if len(validProxies) == 0 {
				continue
			}
			shadows[originalGroup] = shadowProxyGroup{
				Name:    buildShadowProxyGroupName(groupName, originalGroup),
				Proxies: validProxies,
			}
		}
		if len(shadows) == 0 {
			continue
		}

		providerName := buildManagedDeviceProviderName(g, usedProviderNames)
		contexts = append(contexts, deviceRuleContext{
			Shadows: shadows,
			DeviceProvider: DeviceRuleProviderSpec{
				Name:     providerName,
				FileName: providerName + ".yaml",
				Payload:  payload,
			},
		})
	}
	return contexts
}

func buildShadowProxyGroupName(groupName, originalGroup string) string {
	return fmt.Sprintf("%s - %s", groupName, originalGroup)
}

func collectDeviceRuleProviderSpecs(contexts []deviceRuleContext) []DeviceRuleProviderSpec {
	specs := make([]DeviceRuleProviderSpec, 0, len(contexts))
	for _, ctx := range contexts {
		specs = append(specs, ctx.DeviceProvider)
	}
	return specs
}

func upsertManagedDeviceRuleProviders(base map[string]interface{}, specs []DeviceRuleProviderSpec) {
	providers := readRuleProviders(base["rule-providers"])
	for name := range providers {
		if strings.HasPrefix(name, managedDeviceProviderPrefix) {
			delete(providers, name)
		}
	}
	for _, spec := range specs {
		if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.FileName) == "" {
			continue
		}
		providers[spec.Name] = map[string]interface{}{
			"type":     "file",
			"behavior": "ipcidr",
			"path":     fmt.Sprintf(managedDeviceProviderPathTmpl, spec.FileName),
			"interval": 0,
		}
	}
	base["rule-providers"] = providers
}

func readRuleProviders(raw interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	switch providers := raw.(type) {
	case map[string]interface{}:
		for k, v := range providers {
			out[k] = v
		}
	case map[interface{}]interface{}:
		for k, v := range providers {
			key, ok := k.(string)
			if !ok {
				continue
			}
			out[key] = v
		}
	}
	return out
}

func buildManagedDeviceProviderName(group DeviceGroup, used map[string]bool) string {
	candidate := strings.TrimSpace(group.ID)
	if candidate == "" {
		candidate = strings.TrimSpace(group.Name)
	}
	candidate = sanitizeProviderToken(candidate)
	if candidate == "" {
		candidate = "group"
	}
	base := managedDeviceProviderPrefix + candidate
	name := base
	for seq := 2; used[name]; seq++ {
		name = fmt.Sprintf("%s-%d", base, seq)
	}
	used[name] = true
	return name
}

func sanitizeProviderToken(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	builder := strings.Builder{}
	lastDash := false
	for _, r := range raw {
		isAlpha := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isAlpha || isDigit {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func collectAllowedProxyNames(base map[string]interface{}) map[string]bool {
	allowed := map[string]bool{
		"DIRECT": true,
		"REJECT": true,
		"PASS":   true,
	}
	for _, p := range readProxyNames(base["proxies"]) {
		allowed[p] = true
	}
	return allowed
}

func filterProxyList(items []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if !allowed[name] {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func readProxyNames(raw interface{}) []string {
	var out []string
	switch proxies := raw.(type) {
	case []interface{}:
		out = make([]string, 0, len(proxies))
		for _, p := range proxies {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := pm["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			out = append(out, name)
		}
	case []map[string]interface{}:
		out = make([]string, 0, len(proxies))
		for _, pm := range proxies {
			name, _ := pm["name"].(string)
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			out = append(out, name)
		}
	}
	return out
}

func readProxyGroups(raw interface{}) ([]interface{}, map[string]map[string]interface{}) {
	groups := make([]interface{}, 0)
	byName := map[string]map[string]interface{}{}

	switch list := raw.(type) {
	case []interface{}:
		groups = make([]interface{}, 0, len(list))
		byName = make(map[string]map[string]interface{}, len(list))
		for _, item := range list {
			pm, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			groups = append(groups, pm)
			name, _ := pm["name"].(string)
			name = strings.TrimSpace(name)
			if name != "" {
				byName[name] = pm
			}
		}
	case []map[string]interface{}:
		groups = make([]interface{}, 0, len(list))
		byName = make(map[string]map[string]interface{}, len(list))
		for _, pm := range list {
			groups = append(groups, pm)
			name, _ := pm["name"].(string)
			name = strings.TrimSpace(name)
			if name != "" {
				byName[name] = pm
			}
		}
	default:
		return nil, nil
	}

	return groups, byName
}

func upsertProxyGroup(groups *[]interface{}, next map[string]interface{}) {
	name, _ := next["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	for i, g := range *groups {
		existing, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		existingName, _ := existing["name"].(string)
		if strings.TrimSpace(existingName) == name {
			(*groups)[i] = next
			return
		}
	}
	*groups = append(*groups, next)
}

func readRules(raw interface{}) []string {
	switch rules := raw.(type) {
	case []string:
		out := make([]string, 0, len(rules))
		for _, r := range rules {
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(rules))
		for _, item := range rules {
			r, ok := item.(string)
			if !ok {
				continue
			}
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		return out
	default:
		return nil
	}
}

func toInterfaceSlice(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

type ruleLine struct {
	Raw         string
	RuleType    string
	Policy      string
	PolicyIndex int
	Parts       []string
}

func parseRuleLine(raw string) (ruleLine, bool) {
	r := strings.TrimSpace(raw)
	if r == "" {
		return ruleLine{}, false
	}

	parts := splitRuleParts(r)
	if len(parts) < 2 {
		return ruleLine{}, false
	}

	ruleType := strings.ToUpper(parts[0])
	policyIdx := detectPolicyIndex(ruleType, parts)
	if policyIdx < 1 || policyIdx >= len(parts) {
		return ruleLine{}, false
	}

	policy := strings.TrimSpace(parts[policyIdx])
	if policy == "" {
		return ruleLine{}, false
	}

	return ruleLine{
		Raw:         r,
		RuleType:    ruleType,
		Policy:      policy,
		PolicyIndex: policyIdx,
		Parts:       parts,
	}, true
}

func splitRuleParts(rule string) []string {
	rawParts := strings.Split(rule, ",")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		parts = append(parts, strings.TrimSpace(p))
	}
	return parts
}

func detectPolicyIndex(ruleType string, parts []string) int {
	if len(parts) < 2 {
		return -1
	}
	if ruleType == "MATCH" {
		return 1
	}
	idx := len(parts) - 1
	for idx > 0 && isRuleOption(parts[idx]) {
		idx--
	}
	return idx
}

func isRuleOption(part string) bool {
	switch strings.ToLower(strings.TrimSpace(part)) {
	case "no-resolve", "disable-udp", "src", "dst":
		return true
	default:
		return false
	}
}

func firstMatchRuleIndex(rules []string) int {
	for i, rule := range rules {
		parsed, ok := parseRuleLine(rule)
		if !ok {
			continue
		}
		if strings.EqualFold(parsed.RuleType, "MATCH") {
			return i
		}
	}
	return len(rules)
}

func deviceCIDR(d Device) (string, bool) {
	ipText := strings.TrimSpace(d.IP)
	if ipText == "" {
		return "", false
	}
	ip := net.ParseIP(ipText)
	if ip == nil {
		return "", false
	}

	if ip4 := ip.To4(); ip4 != nil {
		prefix := d.Prefix
		if prefix <= 0 {
			prefix = defaultIPv4Prefix
		}
		if prefix < 0 || prefix > 32 {
			return "", false
		}
		return fmt.Sprintf("%s/%d", ip4.String(), prefix), true
	}

	prefix := d.Prefix
	if prefix <= 0 {
		prefix = defaultIPv6Prefix
	}
	if prefix < 0 || prefix > 128 {
		return "", false
	}
	return fmt.Sprintf("%s/%d", ip.String(), prefix), true
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeDeviceGroups(groups []DeviceGroup) []DeviceGroup {
	out := make([]DeviceGroup, 0, len(groups))
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			continue
		}

		devices := make([]Device, 0, len(g.Devices))
		for _, d := range g.Devices {
			ip := strings.TrimSpace(d.IP)
			if ip == "" {
				continue
			}
			devices = append(devices, Device{
				IP:       ip,
				Prefix:   d.Prefix,
				Hostname: strings.TrimSpace(d.Hostname),
			})
		}

		overrides := make([]ProxyGroupOverride, 0, len(g.Overrides))
		for _, ov := range g.Overrides {
			original := strings.TrimSpace(ov.OriginalGroup)
			if original == "" {
				continue
			}
			proxies := make([]string, 0, len(ov.Proxies))
			for _, p := range ov.Proxies {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				proxies = append(proxies, p)
			}
			if len(proxies) == 0 {
				continue
			}
			overrides = append(overrides, ProxyGroupOverride{
				OriginalGroup: original,
				Proxies:       dedupeStrings(proxies),
			})
		}

		out = append(out, DeviceGroup{
			ID:        strings.TrimSpace(g.ID),
			Name:      name,
			Devices:   devices,
			Overrides: overrides,
			Order:     g.Order,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return i < j
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
