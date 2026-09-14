package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func newScheduleStore(t *testing.T) (*Store, []int64) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "sched.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ids := []int64{}
	for _, name := range []string{"a", "b", "c"} {
		id, err := db.CreateTarget(context.Background(), &Target{Name: name, Engine: "fake", Enabled: true, Lane: "wan"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return db, ids
}

func TestCreateAndGetSchedulePreservesTargetOrder(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	want := []int64{ids[2], ids[0], ids[1]}
	id, err := db.CreateSchedule(ctx, &Schedule{
		Name: "nightly", Cron: "0 3 * * *", Enabled: true, Timezone: "Europe/Zurich", TargetIDs: want})
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TargetIDs) != 3 || got.TargetIDs[0] != want[0] || got.TargetIDs[1] != want[1] || got.TargetIDs[2] != want[2] {
		t.Fatalf("target order = %v, want %v", got.TargetIDs, want)
	}
	if got.Name != "nightly" || got.Cron != "0 3 * * *" || !got.Enabled || got.Timezone != "Europe/Zurich" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestCreateScheduleDuplicateNameIsConflict(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	s := &Schedule{Name: "dup", Cron: "@hourly", Enabled: true, Timezone: "UTC", TargetIDs: ids[:1]}
	if _, err := db.CreateSchedule(ctx, s); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSchedule(ctx, s); !errors.Is(err, ErrNameConflict) {
		t.Fatalf("second create err = %v, want ErrNameConflict", err)
	}
}

func TestUpdateScheduleReplacesTargetSet(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	id, err := db.CreateSchedule(ctx, &Schedule{Name: "s", Cron: "@hourly", Enabled: true, Timezone: "UTC", TargetIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateSchedule(ctx, &Schedule{ID: id, Name: "s2", Cron: "*/15 * * * *", Enabled: false,
		Timezone: "UTC", TargetIDs: []int64{ids[1]}}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSchedule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TargetIDs) != 1 || got.TargetIDs[0] != ids[1] || got.Name != "s2" || got.Enabled {
		t.Fatalf("after update: %+v", got)
	}
}

func TestUpdateScheduleMissingIsNotFound(t *testing.T) {
	db, ids := newScheduleStore(t)
	err := db.UpdateSchedule(context.Background(), &Schedule{ID: 999, Name: "x", Cron: "@hourly",
		Timezone: "UTC", TargetIDs: ids[:1]})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteScheduleCascadesTargets(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	id, err := db.CreateSchedule(ctx, &Schedule{Name: "s", Cron: "@hourly", Timezone: "UTC", TargetIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSchedule(ctx, id); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.Read.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedule_targets WHERE schedule_id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("schedule_targets rows = %d, want 0", n)
	}
	if _, err := db.GetSchedule(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestListSchedulesLoadsTargetsForEachRow(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	if _, err := db.CreateSchedule(ctx, &Schedule{Name: "b-second", Cron: "@hourly", Timezone: "UTC", TargetIDs: []int64{ids[0]}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateSchedule(ctx, &Schedule{Name: "a-first", Cron: "@daily", Timezone: "UTC", TargetIDs: []int64{ids[1], ids[2]}}); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "a-first" {
		t.Fatalf("list = %+v, want a-first first", list)
	}
	if len(list[0].TargetIDs) != 2 || len(list[1].TargetIDs) != 1 {
		t.Fatalf("target ids not loaded per row: %+v", list)
	}
}

func TestInsertSkippedRunIsTerminalWithReason(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	sid, err := db.CreateSchedule(ctx, &Schedule{Name: "s", Cron: "@hourly", Timezone: "UTC", TargetIDs: ids[:1]})
	if err != nil {
		t.Fatal(err)
	}
	runID, err := db.InsertSkippedRun(ctx, sid, "previous run still in flight")
	if err != nil {
		t.Fatal(err)
	}
	run, err := db.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "skipped" || run.Trigger != "cron" || run.FinishedAt == nil ||
		run.Error == nil || *run.Error != "previous run still in flight" ||
		run.ScheduleID == nil || *run.ScheduleID != sid {
		t.Fatalf("skipped run = %+v", run)
	}
}

func TestListRunsFiltersByScheduleID(t *testing.T) {
	db, ids := newScheduleStore(t)
	ctx := context.Background()
	sid, err := db.CreateSchedule(ctx, &Schedule{Name: "s", Cron: "@hourly", Timezone: "UTC", TargetIDs: ids[:1]})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRun(ctx, "manual", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRun(ctx, "cron", &sid); err != nil {
		t.Fatal(err)
	}
	runs, _, err := db.ListRuns(ctx, RunFilter{ScheduleID: &sid, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ScheduleID == nil || *runs[0].ScheduleID != sid {
		t.Fatalf("filtered runs = %+v", runs)
	}
}

func TestListSchedulesAttachesLastRunInOneQuery(t *testing.T) {
	s, ctx := openTemp(t), context.Background()
	tid, _ := s.CreateTarget(ctx, &Target{Name: "t", Engine: "fake", Enabled: true, Lane: "wan"})
	id, err := s.CreateSchedule(ctx, &Schedule{Name: "nightly", Cron: "@hourly", Enabled: true, TargetIDs: []int64{tid}})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := s.CreateSchedule(ctx, &Schedule{Name: "quiet", Cron: "@daily", Enabled: true, TargetIDs: []int64{tid}})

	first, _ := s.CreateRun(ctx, "cron", &id)
	if err := s.SetRunStatus(ctx, first, "running", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunStatus(ctx, first, "done", ""); err != nil {
		t.Fatal(err)
	}
	last, _ := s.CreateRun(ctx, "cron", &id)
	if err := s.SetRunStatus(ctx, last, "running", ""); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]Schedule{}
	for _, sc := range list {
		byID[sc.ID] = sc
	}
	got := byID[id].LastRun
	if got == nil || got.Status != "running" || got.StartedAt == "" {
		t.Fatalf("last run = %+v", got)
	}
	if byID[other].LastRun != nil {
		t.Errorf("schedule with no runs should have a nil last_run, got %+v", byID[other].LastRun)
	}
}
