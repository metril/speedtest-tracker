package scheduler

import (
	"testing"
	"time"
)

func TestValidateCronAcceptsFiveFieldsAndDescriptors(t *testing.T) {
	for _, expr := range []string{"*/15 * * * *", "0 3 * * *", "@hourly", "@daily", "@every 30m"} {
		if err := ValidateCron(expr); err != nil {
			t.Errorf("ValidateCron(%q) = %v, want nil", expr, err)
		}
	}
}

func TestValidateCronRejectsGarbageAndSixFields(t *testing.T) {
	for _, expr := range []string{"", "not a cron", "0 0 0 0 0 0", "*/0 * * * *", "@nope"} {
		if err := ValidateCron(expr); err == nil {
			t.Errorf("ValidateCron(%q) = nil, want error", expr)
		}
	}
}

func TestNextFireTimesHonoursTimezone(t *testing.T) {
	from := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	got, err := NextFireTimes("0 3 * * *", "Europe/Zurich", 2, from)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// 03:00 Zurich in September is CEST (UTC+2) => 01:00 UTC.
	if got[0].UTC().Hour() != 1 {
		t.Fatalf("first fire = %s, want 01:00 UTC", got[0].UTC())
	}
	if !got[1].After(got[0]) {
		t.Fatalf("times not increasing: %v", got)
	}
}

func TestNextFireTimesRejectsUnknownTimezone(t *testing.T) {
	if _, err := NextFireTimes("@hourly", "Mars/Olympus", 1, time.Now()); err == nil {
		t.Fatal("want error for unknown timezone")
	}
}

func TestNextFireTimesDefaultsEmptyTimezoneToUTC(t *testing.T) {
	from := time.Date(2026, 9, 13, 0, 30, 0, 0, time.UTC)
	got, err := NextFireTimes("0 * * * *", "", 1, from)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].UTC() != time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC) {
		t.Fatalf("got %s", got[0].UTC())
	}
}
