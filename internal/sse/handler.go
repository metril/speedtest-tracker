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
//
// Flushing goes through http.NewResponseController instead of a Flusher
// type assertion on w directly: middleware (compression, request-id, etc.)
// commonly wraps ResponseWriter in a type that doesn't itself implement
// http.Flusher, and the ResponseController unwraps such wrappers to find
// the underlying Flush method.
func Handler(h *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		head := w.Header()
		head.Set("Content-Type", "text/event-stream")
		head.Set("Cache-Control", "no-cache, no-transform")
		head.Set("Connection", "keep-alive")
		head.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ": connected\n\n")
		if err := rc.Flush(); err != nil {
			return
		}

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
				if err := rc.Flush(); err != nil {
					return
				}
			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
		}
	}
}
