package sse

import (
	"testing"
	"time"
)

func TestHubPublishFanOut(t *testing.T) {
	h := NewHub()
	a, cancelA := h.Subscribe()
	b, cancelB := h.Subscribe()
	defer cancelA()
	defer cancelB()

	if got := h.Subscribers(); got != 2 {
		t.Fatalf("Subscribers = %d, want 2", got)
	}
	h.Publish(h.Marshal(EventRun, map[string]any{"run_id": 7, "status": "running"}))

	for name, ch := range map[string]<-chan Event{"a": a, "b": b} {
		select {
		case ev := <-ch:
			if ev.Type != EventRun {
				t.Errorf("%s: type = %q", name, ev.Type)
			}
			if string(ev.Data) == "" {
				t.Errorf("%s: empty data", name)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: no event", name)
		}
	}
}

func TestHubDropsForSlowSubscriber(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < bufferSize*10; i++ {
			h.Publish(Event{Type: EventProgress, Data: []byte(`{}`)})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
	if got := h.Dropped(); got == 0 {
		t.Error("Dropped = 0, want the overflow counted")
	}
}

func TestHubCancelUnsubscribes(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	cancel()
	cancel() // idempotent

	if got := h.Subscribers(); got != 0 {
		t.Errorf("Subscribers = %d, want 0", got)
	}
	h.Publish(Event{Type: EventResult, Data: []byte(`{}`)})
	if _, open := <-ch; open {
		t.Error("channel should be closed after cancel")
	}
}
