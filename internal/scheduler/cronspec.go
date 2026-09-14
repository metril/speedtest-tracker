// Package scheduler turns stored schedules into cron entries whose only
// job is to enqueue runs on the runner.
package scheduler

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

// specParser accepts the standard 5-field cron syntax plus descriptors
// (@hourly, @daily, @every 30m). Six-field specs with seconds are
// deliberately not accepted: a sub-minute speed test schedule is never
// what the user meant.
var specParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ValidateCron reports whether expr is a usable cron expression.
func ValidateCron(expr string) error {
	if _, err := specParser.Parse(strings.TrimSpace(expr)); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}

// ParseSpec parses expr in the named IANA timezone (empty means UTC). The
// timezone travels with the expression as robfig's CRON_TZ= prefix, so one
// cron instance can host entries in many timezones.
func ParseSpec(expr, tz string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("invalid cron expression: empty")
	}
	if strings.TrimSpace(tz) == "" {
		tz = "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return nil, fmt.Errorf("unknown timezone %q: %w", tz, err)
	}
	sched, err := specParser.Parse("CRON_TZ=" + tz + " " + expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	return sched, nil
}

// NextFireTimes returns the next n fire times strictly after from.
func NextFireTimes(expr, tz string, n int, from time.Time) ([]time.Time, error) {
	sched, err := ParseSpec(expr, tz)
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, n)
	t := from
	for i := 0; i < n; i++ {
		t = sched.Next(t)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out, nil
}
