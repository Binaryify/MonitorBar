package schedule

import (
	"testing"
	"time"
)

var shanghai = time.FixedZone("CST", 8*3600)

func TestAt(t *testing.T) {
	times := Times{DayStart: 7 * 60, NightStart: 19 * 60}
	for _, tc := range []struct {
		hour, minute int
		want         Period
		nextDay      int
		nextHour     int
	}{
		{6, 59, Night, 15, 7},
		{7, 0, Day, 15, 19},
		{18, 59, Day, 15, 19},
		{19, 0, Night, 16, 7},
		{23, 30, Night, 16, 7},
	} {
		now := time.Date(2026, 10, 15, tc.hour, tc.minute, 0, 0, shanghai)
		got, next := times.At(now)
		if got != tc.want || next.Day() != tc.nextDay || next.Hour() != tc.nextHour {
			t.Errorf("%02d:%02d = %s until %s, want %s until day %d %02d:00",
				tc.hour, tc.minute, got, next.Format("02 15:04"), tc.want, tc.nextDay, tc.nextHour)
		}
	}
}

func TestDayOverMidnight(t *testing.T) {
	times := Times{DayStart: 20 * 60, NightStart: 6 * 60}
	for hour, want := range map[int]Period{21: Day, 2: Day, 6: Night, 12: Night} {
		if got, _ := times.At(time.Date(2026, 10, 15, hour, 0, 0, 0, shanghai)); got != want {
			t.Errorf("%02d:00 = %s, want %s", hour, got, want)
		}
	}
}
