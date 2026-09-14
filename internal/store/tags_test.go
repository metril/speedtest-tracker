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

func TestSetResultTagsCancelledContextNotNotFound(t *testing.T) {
	s := openTemp(t)
	tid, _ := s.CreateTarget(context.Background(), &Target{Name: "a", Engine: "fake", Enabled: true, Lane: "wan"})
	rid := insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:00:00.000Z")

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.SetResultTags(cancelled, rid, []string{"x"})
	if err == nil {
		t.Fatal("SetResultTags with cancelled context: want error, got nil")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("SetResultTags with cancelled context = %v, want a non-ErrNotFound error", err)
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

func TestRenameAndDeleteTag(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "t", Engine: "fake", Enabled: true, Lane: "wan"})
	rid := insertResultAt(t, s, tid, "fake", "ok", "2026-09-13T10:00:00.000Z")
	if _, err := s.SetResultTags(ctx, rid, []string{"evening", "wifi"}); err != nil {
		t.Fatal(err)
	}
	tags, _ := s.ListTags(ctx)

	renamed, err := s.RenameTag(ctx, tags[0].ID, "  Morning  ")
	if err != nil {
		t.Fatalf("RenameTag: %v", err)
	}
	if renamed.Name != "morning" {
		t.Errorf("name = %q, want normalised %q", renamed.Name, "morning")
	}
	res, _ := s.GetResult(ctx, rid)
	if len(res.Tags) != 2 || res.Tags[0] != "morning" {
		t.Errorf("result tags = %v", res.Tags)
	}

	if _, err := s.RenameTag(ctx, tags[0].ID, "wifi"); !errors.Is(err, ErrNameConflict) {
		t.Errorf("rename onto an existing name = %v, want ErrNameConflict", err)
	}
	if _, err := s.RenameTag(ctx, 9999, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("rename missing tag = %v, want ErrNotFound", err)
	}

	if err := s.DeleteTag(ctx, tags[0].ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	res, _ = s.GetResult(ctx, rid)
	if len(res.Tags) != 1 || res.Tags[0] != "wifi" {
		t.Errorf("tags after delete = %v", res.Tags)
	}
	if err := s.DeleteTag(ctx, tags[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing tag = %v, want ErrNotFound", err)
	}
}
