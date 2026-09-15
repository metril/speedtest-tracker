package scheduler

import (
	"fmt"
	"time"
)

// overlapWindow is how far ahead overlap detection looks, and
// overlapThreshold how close two fires must be to be called an overlap.
const (
	overlapWindow    = 24 * time.Hour
	overlapThreshold = 60 * time.Second
	maxFiresPerSched = 2000 // guards against @every 1s style expressions
)

// OverlapCandidate is one enabled schedule reduced to what overlap
// detection needs: when it fires and which queues it occupies.
type OverlapCandidate struct {
	ID       int64
	Name     string
	Cron     string
	Timezone string
	Queues   []string
}

// firesWithin lists the schedule's fire times in [from, from+overlapWindow].
func (c OverlapCandidate) firesWithin(from time.Time) []time.Time {
	sched, err := ParseSpec(c.Cron, c.Timezone)
	if err != nil {
		return nil
	}
	end := from.Add(overlapWindow)
	out := []time.Time{}
	for t := sched.Next(from); !t.IsZero() && !t.After(end) && len(out) < maxFiresPerSched; t = sched.Next(t) {
		out = append(out, t)
	}
	return out
}

// sharedQueue returns the first queue both candidates occupy, if any.
func sharedQueue(a, b OverlapCandidate) (string, bool) {
	set := make(map[string]bool, len(a.Queues))
	for _, q := range a.Queues {
		set[q] = true
	}
	for _, q := range b.Queues {
		if set[q] {
			return q, true
		}
	}
	return "", false
}

// FindOverlaps reports, for each other enabled schedule that shares a
// queue with subject and fires within 60s of it in the next 24h, a
// human-readable warning. Overlapping tests on one queue skew each
// other's results, so the UI surfaces these when a schedule is saved.
func FindOverlaps(subject OverlapCandidate, others []OverlapCandidate, from time.Time) []string {
	mine := subject.firesWithin(from)
	if len(mine) == 0 {
		return nil
	}
	warnings := []string{}
	for _, other := range others {
		if other.ID == subject.ID {
			continue
		}
		queue, ok := sharedQueue(subject, other)
		if !ok {
			continue
		}
		theirs := other.firesWithin(from)
		if len(theirs) == 0 {
			continue
		}
		if at, ok := nearestCollision(mine, theirs); ok {
			warnings = append(warnings, fmt.Sprintf(
				"overlaps with schedule %q on queue %q around %s; overlapping tests skew each other's results",
				other.Name, queue, at.UTC().Format(time.RFC3339)))
		}
	}
	return warnings
}

// nearestCollision returns the first time in a that is within
// overlapThreshold of any time in b. Both slices are ascending, so a
// two-pointer walk is enough.
func nearestCollision(a, b []time.Time) (time.Time, bool) {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		d := a[i].Sub(b[j])
		if d < 0 {
			d = -d
		}
		if d <= overlapThreshold {
			return a[i], true
		}
		if a[i].Before(b[j]) {
			i++
		} else {
			j++
		}
	}
	return time.Time{}, false
}
