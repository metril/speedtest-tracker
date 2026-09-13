package store

import (
	"context"
	"errors"
	"testing"
)

func TestSetResultTags(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	rid := insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:00:00.000Z")

	got, err := s.SetResultTags(ctx, rid, []string{"night", "isp-issue", "night"})
	if err != nil {
		t.Fatalf("SetResultTags: %v", err)
	}
	if len(got) != 2 || got[0] != "isp-issue" || got[1] != "night" {
		t.Fatalf("tags = %v, want sorted deduped [isp-issue night]", got)
	}

	res, err := s.GetResult(ctx, rid)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if len(res.Tags) != 2 || res.Tags[0] != "isp-issue" {
		t.Errorf("result tags = %v", res.Tags)
	}

	// Replacing the set removes the dropped association but keeps the tag row.
	if _, err := s.SetResultTags(ctx, rid, []string{"night"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	res, _ = s.GetResult(ctx, rid)
	if len(res.Tags) != 1 || res.Tags[0] != "night" {
		t.Errorf("after replace = %v", res.Tags)
	}
	tags, err := s.ListTags(ctx)
	if err != nil || len(tags) != 2 {
		t.Errorf("ListTags = %v %v, want both tag rows kept", tags, err)
	}

	if _, err := s.SetResultTags(ctx, rid, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	res, _ = s.GetResult(ctx, rid)
	if len(res.Tags) != 0 {
		t.Errorf("after clear = %v", res.Tags)
	}
	if _, err := s.SetResultTags(ctx, 999, []string{"x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown result = %v, want ErrNotFound", err)
	}
}

func TestListResultsIncludesTags(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	r1 := insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:00:00.000Z")
	insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T11:00:00.000Z")
	if _, err := s.SetResultTags(ctx, r1, []string{"tagged"}); err != nil {
		t.Fatal(err)
	}

	list, _, err := s.ListResults(ctx, ResultFilter{})
	if err != nil || len(list) != 2 {
		t.Fatalf("ListResults = %v %v", list, err)
	}
	for _, r := range list {
		want := 0
		if r.ID == r1 {
			want = 1
		}
		if len(r.Tags) != want {
			t.Errorf("result %d tags = %v, want %d", r.ID, r.Tags, want)
		}
	}
}
