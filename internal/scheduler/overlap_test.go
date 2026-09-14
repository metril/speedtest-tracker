package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestFindOverlapsWarnsOnSameLaneNearSimultaneousFire(t *testing.T) {
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	subject := OverlapCandidate{ID: 1, Name: "hourly-wan", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"wan"}}
	others := []OverlapCandidate{
		{ID: 2, Name: "also-hourly", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"wan"}},
	}
	got := FindOverlaps(subject, others, from)
	if len(got) != 1 || !strings.Contains(got[0], "also-hourly") || !strings.Contains(got[0], "wan") {
		t.Fatalf("warnings = %v", got)
	}
}

func TestFindOverlapsIgnoresDifferentLanes(t *testing.T) {
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	subject := OverlapCandidate{ID: 1, Name: "wan", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"wan"}}
	others := []OverlapCandidate{{ID: 2, Name: "lan", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"lan"}}}
	if got := FindOverlaps(subject, others, from); len(got) != 0 {
		t.Fatalf("warnings = %v, want none", got)
	}
}

func TestFindOverlapsIgnoresFiresMoreThan60sApart(t *testing.T) {
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	subject := OverlapCandidate{ID: 1, Name: "on-the-hour", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"wan"}}
	others := []OverlapCandidate{
		{ID: 2, Name: "half-past", Cron: "30 * * * *", Timezone: "UTC", Lanes: []string{"wan"}},
	}
	if got := FindOverlaps(subject, others, from); len(got) != 0 {
		t.Fatalf("warnings = %v, want none", got)
	}
}

func TestFindOverlapsSkipsSelfAndInvalidCron(t *testing.T) {
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	subject := OverlapCandidate{ID: 1, Name: "s", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"wan"}}
	others := []OverlapCandidate{
		{ID: 1, Name: "s", Cron: "0 * * * *", Timezone: "UTC", Lanes: []string{"wan"}},
		{ID: 3, Name: "broken", Cron: "nonsense", Timezone: "UTC", Lanes: []string{"wan"}},
	}
	if got := FindOverlaps(subject, others, from); len(got) != 0 {
		t.Fatalf("warnings = %v, want none", got)
	}
}

func TestFindOverlapsReturnsNothingForInvalidSubject(t *testing.T) {
	if got := FindOverlaps(OverlapCandidate{Cron: "nonsense"}, nil, time.Now()); len(got) != 0 {
		t.Fatalf("warnings = %v", got)
	}
}
