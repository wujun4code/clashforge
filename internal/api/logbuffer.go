package api

import (
	"sync"

	"github.com/rs/zerolog"
)

// LogEntry is a single captured log line.
type LogEntry struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Ts    int64  `json:"ts"`
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

// Write implements zerolog.LevelWriter so it can be used as a zerolog hook destination.
func (b *LogBuffer) Write(p []byte) (int, error)                        { return len(p), nil }
func (b *LogBuffer) WriteLevel(l zerolog.Level, p []byte) (int, error) { return len(p), nil }

// Add appends a log entry (called from zerolog hook).
func (b *LogBuffer) Add(level, msg string, ts int64) {
	b.mu.Lock()
	b.buf[b.head] = LogEntry{Level: level, Msg: msg, Ts: ts}
	b.head = (b.head + 1) % b.cap_
	if b.size < b.cap_ {
		b.size++
	}
	b.mu.Unlock()
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

// ZerologHook is a zerolog hook that feeds entries into a LogBuffer.
type ZerologHook struct {
	buf *LogBuffer
}

func NewZerologHook(buf *LogBuffer) ZerologHook { return ZerologHook{buf: buf} }

func (h ZerologHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	h.buf.Add(level.String(), msg, timeNowUnix())
}

// timeNowUnix is a package-level var so tests can override it.
var timeNowUnix = func() int64 {
	return zerolog.TimestampFunc()().Unix()
}
