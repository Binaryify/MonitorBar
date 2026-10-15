// Package schedule decides whether it is day or night by the times the
// user set, so the app can switch scenes on its own.
package schedule

import "time"

// Period is day or night.
type Period string

const (
	Day   Period = "day"
	Night Period = "night"
)

// Times are when day and night start, in minutes after midnight.
type Times struct {
	DayStart, NightStart int
}

// At tells the period at a moment, and when the next one begins. Day may
// run over midnight, as from 20:00 to 06:00 for a night-shift worker.
func (t Times) At(now time.Time) (Period, time.Time) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	next := func(minutes int) time.Time {
		at := midnight.Add(time.Duration(minutes) * time.Minute)
		if !at.After(now) {
			at = midnight.AddDate(0, 0, 1).Add(time.Duration(minutes) * time.Minute)
		}
		return at
	}
	m := now.Hour()*60 + now.Minute()
	isDay := m >= t.DayStart && m < t.NightStart
	if t.DayStart > t.NightStart {
		isDay = m >= t.DayStart || m < t.NightStart
	}
	if isDay {
		return Day, next(t.NightStart)
	}
	return Night, next(t.DayStart)
}
