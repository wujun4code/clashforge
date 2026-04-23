package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/config"
)

// ActiveSource records which config source is currently active.
type ActiveSource struct {
	Type     string `json:"type"`               // "file" or "subscription"
	Filename string `json:"filename,omitempty"` // set when Type == "file"
	SubID    string `json:"sub_id,omitempty"`   // set when Type == "subscription"
	SubName  string `json:"sub_name,omitempty"` // display name for subscription
}

// SourceFile represents a saved source config file on disk.
type SourceFile struct {
	Filename  string    `json:"filename"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
	Active    bool      `json:"active"`
}

func sourcesDirPath(dataDir string) string {
	return filepath.Join(dataDir, "sources")
}

func activeSourceFilePath(dataDir string) string {
	return filepath.Join(dataDir, "active_source.json")
}

func readActiveSource(dataDir string) (*ActiveSource, error) {
	data, err := os.ReadFile(activeSourceFilePath(dataDir))
	if err != nil {
		return nil, err
	}
	var as ActiveSource
	if err := json.Unmarshal(data, &as); err != nil {
		return nil, err
	}
	return &as, nil
}

func writeActiveSource(dataDir string, as ActiveSource) error {
	data, err := json.MarshalIndent(as, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(activeSourceFilePath(dataDir), data, 0o644)
}

// nextPastedFilename returns "YYYYMMDD_vN.yaml" with N auto-incremented.
func nextPastedFilename(dir string) string {
	date := time.Now().Format("20060102")
	for n := 1; n <= 9999; n++ {
		name := fmt.Sprintf("%s_v%d.yaml", date, n)
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
	return fmt.Sprintf("%s_v1.yaml", date)
}

// sanitizeSourceFilename makes a filename safe (alphanumeric, CJK, dots, dashes, underscores).
func sanitizeSourceFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_',
			r >= '\u4e00' && r <= '\u9fff':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "config"
	}
	runes := []rune(result)
	if len(runes) > 80 {
		result = string(runes[:80])
	}
	return result
}

func handleGetSources(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dataDir := deps.Config.Core.DataDir
		dir := sourcesDirPath(dataDir)
		_ = os.MkdirAll(dir, 0o755)

		activeSource, _ := readActiveSource(dataDir)

		entries, _ := os.ReadDir(dir)
		files := make([]SourceFile, 0)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			active := activeSource != nil &&
				activeSource.Type == "file" &&
				activeSource.Filename == name
			files = append(files, SourceFile{
				Filename:  name,
				CreatedAt: info.ModTime(),
				SizeBytes: info.Size(),
				Active:    active,
			})
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"files":         files,
			"active_source": activeSource,
		})
	}
}

func handleSaveSource(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content       string `json:"content"`
			SuggestedName string `json:"suggested_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			Err(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
			return
		}
		if strings.TrimSpace(body.Content) == "" {
			Err(w, http.StatusBadRequest, "EMPTY_CONTENT", "content is required")
			return
		}
		if err := config.ValidateYAML([]byte(body.Content)); err != nil {
			Err(w, http.StatusBadRequest, "YAML_PARSE_ERROR", err.Error())
			return
		}

		dataDir := deps.Config.Core.DataDir
		dir := sourcesDirPath(dataDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			Err(w, http.StatusInternalServerError, "MKDIR_FAILED", err.Error())
			return
		}

		// Deduplicate: if a file with identical content already exists, reuse it.
		incomingHash := sha256.Sum256([]byte(body.Content))
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
				continue
			}
			existing, err := os.ReadFile(filepath.Join(dir, n))
			if err != nil {
				continue
			}
			if sha256.Sum256(existing) == incomingHash {
				JSON(w, http.StatusOK, map[string]string{"filename": n})
				return
			}
		}

		var filename string
		if body.SuggestedName != "" {
			base := sanitizeSourceFilename(body.SuggestedName)
			if !strings.HasSuffix(base, ".yaml") && !strings.HasSuffix(base, ".yml") {
				base += ".yaml"
			}
			candidate := base
			for i := 2; ; i++ {
				if _, err := os.Stat(filepath.Join(dir, candidate)); os.IsNotExist(err) {
					break
				}
				ext := filepath.Ext(base)
				stem := strings.TrimSuffix(base, ext)
				candidate = fmt.Sprintf("%s_%d%s", stem, i, ext)
			}
			filename = candidate
		} else {
			filename = nextPastedFilename(dir)
		}

		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
			Err(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]string{"filename": filename})
	}
}

func handleGetSourceFile(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := chi.URLParam(r, "filename")
		if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "..") || strings.Contains(filename, string(filepath.Separator)) {
			Err(w, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
			return
		}
		dir := sourcesDirPath(deps.Config.Core.DataDir)
		path := filepath.Join(dir, filename)
		// Ensure the resolved path is within the sources dir
		clean := filepath.Clean(path)
		if !strings.HasPrefix(clean, filepath.Clean(dir)) {
			Err(w, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
			return
		}
		data, err := os.ReadFile(clean)
		if err != nil {
			if os.IsNotExist(err) {
				Err(w, http.StatusNotFound, "NOT_FOUND", "file not found")
				return
			}
			Err(w, http.StatusInternalServerError, "READ_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]string{
			"filename": filename,
			"content":  string(data),
		})
	}
}

func handleDeleteSourceFile(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := chi.URLParam(r, "filename")
		if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "..") || strings.Contains(filename, string(filepath.Separator)) {
			Err(w, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
			return
		}
		dir := sourcesDirPath(deps.Config.Core.DataDir)
		path := filepath.Join(dir, filename)
		clean := filepath.Clean(path)
		if !strings.HasPrefix(clean, filepath.Clean(dir)) {
			Err(w, http.StatusBadRequest, "INVALID_FILENAME", "invalid filename")
			return
		}
		if err := os.Remove(clean); err != nil {
			if os.IsNotExist(err) {
				Err(w, http.StatusNotFound, "NOT_FOUND", "file not found")
				return
			}
			Err(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func handleGetActiveSource(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		as, err := readActiveSource(deps.Config.Core.DataDir)
		if err != nil {
			JSON(w, http.StatusOK, map[string]interface{}{"active_source": nil})
			return
		}
		JSON(w, http.StatusOK, map[string]interface{}{"active_source": as})
	}
}

func handleSetActiveSource(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var as ActiveSource
		if err := json.NewDecoder(r.Body).Decode(&as); err != nil {
			Err(w, http.StatusBadRequest, "PARSE_ERROR", err.Error())
			return
		}
		if as.Type != "file" && as.Type != "subscription" {
			Err(w, http.StatusBadRequest, "INVALID_TYPE", "type must be 'file' or 'subscription'")
			return
		}
		if err := writeActiveSource(deps.Config.Core.DataDir, as); err != nil {
			Err(w, http.StatusInternalServerError, "WRITE_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]bool{"updated": true})
	}
}
