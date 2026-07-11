package ast

import "time"

// DateAnchor is either a concrete date or the symbolic "now" (resolved at
// compile time, not parse time — this is what makes a stored "last 30 days"
// smart collection roll).
type DateAnchor struct {
	Now  bool      // true = resolve to `now` at compile time
	Date time.Time // meaningful only when Now is false
}

// DateDuration is a calendar-aware offset with time-of-day precision.
// Calendar components apply via time.AddDate ("last 3 months" means 3 calendar
// months, not 90 days); time components apply as exact clock offsets. Negative
// values look backward ("last N"). The wire form is an ISO 8601 duration
// string (duration.go) — components never mix signs.
type DateDuration struct {
	Years   int
	Months  int
	Days    int
	Hours   int
	Minutes int
	Seconds int
}

// IsZero reports whether the duration is empty (no offset).
func (d DateDuration) IsZero() bool {
	return d.Years == 0 && d.Months == 0 && d.Days == 0 &&
		d.Hours == 0 && d.Minutes == 0 && d.Seconds == 0
}

// DateValue is the value for dateRange leaves: a half-open interval
// [anchor, anchor+duration) — or [anchor+duration, anchor) when duration is
// negative ("last 30 days" = anchor "now", duration -30d).
type DateValue struct {
	Anchor   DateAnchor
	Duration DateDuration
}

// Resolve computes the half-open [start, end) interval at compile time.
// Machine-local timezone for day boundaries ("today" means the user's today).
func (d *DateValue) Resolve(now time.Time) (start, end time.Time) {
	var anchor time.Time
	if d.Anchor.Now {
		anchor = now
	} else {
		anchor = d.Anchor.Date
	}

	other := anchor.AddDate(d.Duration.Years, d.Duration.Months, d.Duration.Days).
		Add(time.Duration(d.Duration.Hours)*time.Hour +
			time.Duration(d.Duration.Minutes)*time.Minute +
			time.Duration(d.Duration.Seconds)*time.Second)
	if other.Before(anchor) {
		return other, anchor
	}
	return anchor, other
}
