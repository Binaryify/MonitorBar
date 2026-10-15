// Package usage keeps daily screen time: how long the monitor was lit, and
// how long someone was using it.
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Interval is how often Record should be called.
const Interval = 30 * time.Second

// activeWithin is how recent input must be for a sample to count as use.
const activeWithin = 2 * time.Minute

const keepDays = 60

type Day struct {
	Day    string `json:"day"`    // 2006-01-02
	Awake  int    `json:"awake"`  // seconds lit
	Active int    `json:"active"` // seconds with keyboard or mouse input
}

// Tracker samples the monitor and stores totals in a JSON file.
type Tracker struct {
	path string
	days map[string]Day
	now  func() time.Time
}

func Open(path string) *Tracker {
	t := &Tracker{path: path, days: map[string]Day{}, now: time.Now}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &t.days)
	}
	return t
}

// Record adds one sample: nothing while the monitor sleeps.
func (t *Tracker) Record(awake bool, idle time.Duration) {
	if !awake {
		return
	}
	key := t.now().Format(time.DateOnly)
	d := t.days[key]
	d.Day = key
	d.Awake += int(Interval.Seconds())
	if idle < activeWithin {
		d.Active += int(Interval.Seconds())
	}
	t.days[key] = d
	keys := make([]string, 0, len(t.days))
	for k := range t.days {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, old := range keys[:max(0, len(keys)-keepDays)] {
		delete(t.days, old)
	}
	t.save()
}

// Recent returns the last n days, oldest first, with empty days included.
func (t *Tracker) Recent(n int) []Day {
	out := make([]Day, n)
	today := t.now()
	for i := range n {
		key := today.AddDate(0, 0, i-n+1).Format(time.DateOnly)
		out[i] = t.days[key]
		out[i].Day = key
	}
	return out
}

func (t *Tracker) save() {
	data, err := json.Marshal(t.days)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(t.path), 0o755)
	_ = os.WriteFile(t.path, data, 0o644)
}
