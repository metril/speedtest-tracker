package store

import (
	"context"
	"errors"
	"testing"
)

func TestEachResultStreamsNewestFirstWithFilters(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	b, _ := s.CreateTarget(ctx, &Target{Name: "b", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, a, "fake", "failed", "2026-09-13T11:00:00.000Z")
	insertResultAt(t, s, b, "fake", "ok", "2026-09-13T12:00:00.000Z")

	var seen []string
	if err := s.EachResult(ctx, ResultFilter{TargetID: &a}, func(r *Result) error {
		seen = append(seen, r.StartedAt)
		return nil
	}); err != nil {
		t.Fatalf("EachResult: %v", err)
	}
	if len(seen) != 2 || seen[0] != "2026-09-13T11:00:00.000Z" {
		t.Fatalf("seen = %v", seen)
	}
}

func TestEachResultStopsOnCallbackError(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	a, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, a, "fake", "ok", "2026-09-13T11:00:00.000Z")

	boom := errors.New("client gone")
	n := 0
	err := s.EachResult(ctx, ResultFilter{}, func(*Result) error { n++; return boom })
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("err = %v after %d rows", err, n)
	}
}
