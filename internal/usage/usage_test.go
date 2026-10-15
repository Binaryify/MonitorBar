package usage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.json")
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)
	tr := Open(path)
	tr.now = func() time.Time { return now }

	tr.Record(true, time.Second)    // awake and used
	tr.Record(true, 10*time.Minute) // awake, idle
	tr.Record(false, 0)             // asleep: ignored
	now = now.AddDate(0, 0, -2)
	tr.Record(true, 0)

	reopened := Open(path)
	reopened.now = func() time.Time { return time.Date(2026, 10, 14, 23, 0, 0, 0, time.Local) }
	days := reopened.Recent(3)
	want := []Day{{"2026-10-12", 30, 30}, {"2026-10-13", 0, 0}, {"2026-10-14", 60, 30}}
	for i := range want {
		if days[i] != want[i] {
			t.Errorf("day %d = %+v, want %+v", i, days[i], want[i])
		}
	}
}

func TestKeepsSixtyDays(t *testing.T) {
	tr := Open(filepath.Join(t.TempDir(), "usage.json"))
	day := time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
	for i := range 70 {
		tr.now = func() time.Time { return day.AddDate(0, 0, i) }
		tr.Record(true, 0)
	}
	if len(tr.days) != keepDays {
		t.Fatalf("kept %d days, want %d", len(tr.days), keepDays)
	}
	if _, ok := tr.days["2026-01-01"]; ok {
		t.Error("the oldest day was kept")
	}
}
