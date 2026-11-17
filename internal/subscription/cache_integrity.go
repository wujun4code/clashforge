package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// A commit marker is published last. A pending/invalid marker prevents a crash
// between the independent raw and node renames from looking like legacy input.
// Hashes bind both files to one generation; node-only sources explicitly omit raw.
type cacheCommit struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Nodes   string `json:"nodes_sha256"`
	Raw     string `json:"raw_sha256,omitempty"`
}

func cacheHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func atomicCacheFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	// Persist each rename before proceeding, especially the pending marker:
	// power loss must not expose newly replaced files as markerless legacy data.
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (m *Manager) writeCache(id string, nodes []ProxyNode, content []byte, keepRaw bool) error {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	// Classify mappings independently of semantic validity: an invalid full
	// config must never be downgraded to nodes or retire the previous raw input.
	var root yaml.Node
	mapping := yaml.Unmarshal(content, &root) == nil && len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode
	full := validYAMLMapping(content)
	if mapping && !full {
		return errors.New("subscription cache: invalid YAML mapping")
	}
	dir := filepath.Join(m.dataDir, "cache")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.New("subscription cache: cannot create storage")
	}
	data, err := json.Marshal(nodes)
	if err != nil {
		return errors.New("subscription cache: cannot encode nodes")
	}
	base := filepath.Join(dir, id)
	marker := cacheCommit{Version: 1, Kind: "nodes", Nodes: cacheHash(data)}
	if full {
		marker.Kind = "full"
	}
	if full || keepRaw {
		marker.Raw = cacheHash(content)
	}
	// Publish pending before touching either file, including the first migration.
	if err := atomicCacheFile(base+".commit.json", []byte(`{"version":0}`)); err != nil {
		return errors.New("subscription cache: cannot begin commit")
	}
	if full || keepRaw {
		if err := atomicCacheFile(base+".raw.yaml", content); err != nil {
			return errors.New("subscription cache: cannot persist raw input")
		}
	} else if err := os.Remove(base + ".raw.yaml"); err != nil && !os.IsNotExist(err) {
		return errors.New("subscription cache: cannot retire raw input")
	}
	if err := atomicCacheFile(base+".json", data); err != nil {
		return errors.New("subscription cache: cannot persist nodes")
	}
	encoded, _ := json.Marshal(marker)
	if err := atomicCacheFile(base+".commit.json", encoded); err != nil {
		return errors.New("subscription cache: cannot finish commit")
	}
	// Sync rename directory entries as well as staged file contents.
	d, err := os.Open(dir)
	if err != nil {
		return errors.New("subscription cache: cannot sync commit")
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return errors.New("subscription cache: cannot sync commit")
	}
	return nil
}

func validYAMLMapping(data []byte) bool {
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) != 1 {
		return false
	}
	if root.Content[0].Kind != yaml.MappingNode || len(root.Content[0].Content) == 0 {
		return false
	}
	// Match GenerateFromBase's decoder, including nested duplicate-key checks.
	// Never return parser errors: they may contain private mapping keys.
	var decoded map[string]interface{}
	return yaml.Unmarshal(data, &decoded) == nil && len(decoded) > 0
}

// GenerationInput is deliberately strict, unlike the compatibility view APIs.
// Legacy full YAML + JSON is supported. Rawless legacy JSON is ambiguous: it
// cannot prove whether rules/groups were lost, so it requires a fresh sync.
func (m *Manager) GenerationInput(id string) ([]ProxyNode, []byte, error) {
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()
	fail := func() ([]ProxyNode, []byte, error) {
		return nil, nil, errors.New("subscription persistent input incomplete or corrupt; synchronize source again")
	}
	base := filepath.Join(m.dataDir, "cache", id)
	data, err := os.ReadFile(base + ".json")
	if err != nil {
		return fail()
	}
	var nodes []ProxyNode
	if json.Unmarshal(data, &nodes) != nil {
		return fail()
	}
	raw, rawErr := os.ReadFile(base + ".raw.yaml")
	encoded, err := os.ReadFile(base + ".commit.json")
	if os.IsNotExist(err) {
		if rawErr != nil || !validYAMLMapping(raw) {
			return fail()
		}
	} else {
		var marker cacheCommit
		if err != nil || json.Unmarshal(encoded, &marker) != nil || marker.Version != 1 || marker.Nodes != cacheHash(data) {
			return fail()
		}
		if marker.Kind != "full" && marker.Kind != "nodes" {
			return fail()
		}
		if marker.Raw != "" {
			if rawErr != nil || marker.Raw != cacheHash(raw) {
				return fail()
			}
		} else if !os.IsNotExist(rawErr) {
			return fail()
		}
		if marker.Kind == "nodes" {
			raw = nil
		} else if marker.Raw == "" || !validYAMLMapping(raw) {
			return fail()
		}
	}
	for i := range nodes {
		nodes[i].SourceSubID = id
	}
	return nodes, raw, nil
}
