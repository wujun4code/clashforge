package api

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// LogEntry is a single captured log line, including all structured fields.
type LogEntry struct {
	Level  string                 `json:"level"`
	Msg    string                 `json:"msg"`
	Ts     int64                  `json:"ts"`
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// LogBuffer is a fixed-size ring buffer that captures zerolog output.
type LogBuffer struct {
	mu   sync.Mutex
	buf  []LogEntry
	cap_ int
	head int
	size int
}

func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{buf: make([]LogEntry, capacity), cap_: capacity}
}

// Write satisfies io.Writer (no-op: we only capture via WriteLevel).
func (b *LogBuffer) Write(p []byte) (int, error) { return len(p), nil }

// WriteLevel is called by zerolog.MultiLevelWriter with the full raw JSON event.
// It parses the JSON to extract level, message, timestamp, and all extra fields.
func (b *LogBuffer) WriteLevel(l zerolog.Level, p []byte) (int, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(p, &raw); err != nil {
		// Fallback: store the raw line as the message if JSON is malformed.
		b.add(LogEntry{Level: l.String(), Msg: string(p), Ts: time.Now().Unix()})
		return len(p), nil
	}

	entry := LogEntry{
		Level:  l.String(),
		Ts:     time.Now().Unix(),
		Fields: make(map[string]interface{}),
	}

	// Extract well-known fields.
	if r, ok := raw[zerolog.MessageFieldName]; ok {
		_ = json.Unmarshal(r, &entry.Msg)
	} else if r, ok := raw["msg"]; ok {
		_ = json.Unmarshal(r, &entry.Msg)
	}
	if r, ok := raw[zerolog.LevelFieldName]; ok {
		var lvl string
		if json.Unmarshal(r, &lvl) == nil && lvl != "" {
			entry.Level = lvl
		}
	}
	if r, ok := raw[zerolog.TimestampFieldName]; ok {
		var ts float64
		if json.Unmarshal(r, &ts) == nil {
			entry.Ts = int64(ts)
		}
	}

	// Collect all remaining fields as structured metadata.
	skip := map[string]bool{
		zerolog.MessageFieldName:   true,
		zerolog.LevelFieldName:     true,
		zerolog.TimestampFieldName: true,
		"msg":                      true,
	}
	for k, v := range raw {
		if skip[k] {
			continue
		}
		var val interface{}
		if json.Unmarshal(v, &val) == nil {
			entry.Fields[k] = val
		}
	}
	if len(entry.Fields) == 0 {
		entry.Fields = nil
	}

	b.add(entry)
	return len(p), nil
}

func (b *LogBuffer) add(entry LogEntry) {
	b.mu.Lock()
	b.buf[b.head] = entry
	b.head = (b.head + 1) % b.cap_
	if b.size < b.cap_ {
		b.size++
	}
	b.mu.Unlock()
}

// Add appends a log entry directly (kept for backward compatibility).
func (b *LogBuffer) Add(level, msg string, ts int64) {
	b.add(LogEntry{Level: level, Msg: msg, Ts: ts})
}

// Recent returns up to n log entries, newest last.
func (b *LogBuffer) Recent(n int) []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n <= 0 || b.size == 0 {
		return nil
	}
	if n > b.size {
		n = b.size
	}
	out := make([]LogEntry, n)
	start := (b.head - n + b.cap_) % b.cap_
	for i := 0; i < n; i++ {
		out[i] = b.buf[(start+i)%b.cap_]
	}
	return out
}

// ZerologHook is kept for API compatibility but is a no-op when MultiLevelWriter is used.
type ZerologHook struct{ buf *LogBuffer }

func NewZerologHook(buf *LogBuffer) ZerologHook { return ZerologHook{buf: buf} }

// Run is a no-op: structured fields are now captured via WriteLevel JSON parsing.
func (h ZerologHook) Run(_ *zerolog.Event, _ zerolog.Level, _ string) {}

var timeNowUnix = func() int64 { return time.Now().Unix() }
