// Package sse fans server-side events out to browser clients. Publishes
// never block: a subscriber that cannot keep up loses events instead of
// stalling a running speed test.
package sse

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// Event types carried on the wire.
const (
	EventProgress = "progress"
	EventResult   = "result"
	EventRun      = "run"
)

// bufferSize is the per-subscriber queue depth.
const bufferSize = 64

// Event is one SSE message: an event name and its already-encoded payload.
type Event struct {
	Type string
	Data json.RawMessage
}

// Hub is a fan-out broker of Events.
type Hub struct {
	mu      sync.Mutex
	subs    map[int]chan Event
	next    int
	dropped int64
}

// NewHub returns an empty Hub.
func NewHub() *Hub {
	return &Hub{subs: map[int]chan Event{}}
}

// Subscribe returns a buffered event channel and a cancel func that
// unsubscribes and closes it. cancel is idempotent.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, bufferSize)
	h.mu.Lock()
	id := h.next
	h.next++
	h.subs[id] = ch
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if c, ok := h.subs[id]; ok {
				delete(h.subs, id)
				close(c)
			}
		})
	}
}

// Publish delivers e to every subscriber with room, dropping it for those
// without. It never blocks.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- e:
		default:
			h.dropped++
		}
	}
}

// Marshal encodes payload into an Event, logging and falling back to an
// empty-object event if encoding fails (never a reason to fail a test run).
func (h *Hub) Marshal(eventType string, payload any) Event {
	data, err := json.Marshal(payload)
	if err != nil {
		slog.Default().Warn("sse: marshal event payload failed, emitting empty object",
			"event_type", eventType, "error", err)
		return Event{Type: eventType, Data: json.RawMessage(`{}`)}
	}
	return Event{Type: eventType, Data: data}
}

// Subscribers reports the current subscriber count.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Dropped reports how many per-subscriber deliveries were dropped.
func (h *Hub) Dropped() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dropped
}
