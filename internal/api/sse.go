package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEBroker manages Server-Sent Events connections.
type SSEBroker struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

// NewSSEBroker creates a broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{clients: make(map[chan string]struct{})}
}

// Publish sends an event to all connected clients.
func (b *SSEBroker) Publish(eventType string, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, payload)
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// client is slow; skip this event rather than blocking
		}
	}
}

// Handler returns an http.HandlerFunc for SSE streaming.
func (b *SSEBroker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := make(chan string, 64)
		b.mu.Lock()
		b.clients[ch] = struct{}{}
		b.mu.Unlock()

		defer func() {
			b.mu.Lock()
			delete(b.clients, ch)
			b.mu.Unlock()
		}()

		// Send initial heartbeat
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg := <-ch:
				fmt.Fprint(w, msg)
				flusher.Flush()
			case <-ticker.C:
				// Keepalive comment
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
