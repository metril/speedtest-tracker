package sse

import (
	"fmt"
	"net/http"
	"time"
)

// keepaliveInterval is how often a comment line is sent so proxies and
// browsers keep an idle stream open.
const keepaliveInterval = 15 * time.Second

// Handler streams hub events to one client. It must be mounted outside any
// request-timeout middleware.
func Handler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		head := w.Header()
		head.Set("Content-Type", "text/event-stream")
		head.Set("Cache-Control", "no-cache, no-transform")
		head.Set("Connection", "keep-alive")
		head.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		events, cancel := h.Subscribe()
		defer cancel()

		ticker := time.NewTicker(keepaliveInterval)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case ev, open := <-events:
				if !open {
					return
				}
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, ev.Data); err != nil {
					return
				}
				flusher.Flush()
			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
