package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// sseHub manages connected SSE clients.
type sseHub struct {
	mu      sync.RWMutex
	clients map[chan<- string]struct{}
}

var globalSSEHub = &sseHub{
	clients: make(map[chan<- string]struct{}),
}

func (h *sseHub) subscribe() (ch chan string, cancel func()) {
	ch = make(chan string, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	cancel = func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

func (h *sseHub) broadcast(eventType string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, payload)
	h.mu.RLock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // drop if client buffer is full (slow consumer)
		}
	}
	h.mu.RUnlock()
}

// broadcastSSE sends a named SSE event to all connected clients.
func broadcastSSE(eventType string, data any) {
	globalSSEHub.broadcast(eventType, data)
}

// sseEvents handles GET /api/v1/events
// Streams named events to the client using the text/event-stream protocol.
func (s *Server) sseEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := globalSSEHub.subscribe()
	defer cancel()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
