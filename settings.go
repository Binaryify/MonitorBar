package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are the choices kept between launches, in a JSON file.
type settings struct {
	SoftBrightness float64 `json:"softBrightness"` // gamma %, 20...100
	Warmth         float64 `json:"warmth"`         // 0...100
	Tab            string  `json:"tab"`
	NetworkHost    string  `json:"networkHost"` // the display found last time
	LaunchedBefore bool    `json:"launchedBefore"`

	// Switching to the day and night scenes at set times.
	AutoSwitch bool   `json:"autoSwitch"`
	DayStart   string `json:"dayStart"` // "07:00"
	NightStart string `json:"nightStart"`
	DayScene   string `json:"dayScene"`
	NightScene string `json:"nightScene"`
	AutoPeriod string `json:"autoPeriod"` // the period whose scene was applied last

	// Following the built-in display's brightness over DDC/CI (MacBooks).
	FollowBuiltin bool    `json:"followBuiltin"`
	FollowMin     float64 `json:"followMin"` // the external backlight at the built-in's darkest, %
	FollowMax     float64 `json:"followMax"` // and at its brightest

	// Shortcuts the user recorded, by id: absent for the default keys, ""
	// for none.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
	// The input this Mac is plugged into, for the shortcut that switches back
	// to it (0 before the monitor told).
	HomeInput int `json:"homeInput,omitempty"`
	// Taking the keyboard's brightness and volume keys (macOS).
	MediaKeys bool `json:"mediaKeys"`
	// The language chosen in the panel, "" to follow the system's.
	Language string `json:"language,omitempty"`
	path     string
}

// dataDir is where MonitorBar keeps its files.
func dataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "MonitorBar")
}

func loadSettings() *settings {
	s := &settings{SoftBrightness: 100, DayStart: "07:00", NightStart: "19:00", DayScene: "day", NightScene: "night",
		FollowMin: 10, FollowMax: 100,
		path: filepath.Join(dataDir(), "settings.json")}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	// Before 1.7 the scenes were kept by their Chinese names.
	legacy := map[string]string{"日间": "day", "阅读": "reading", "观影": "movie", "夜间": "night"}
	for _, id := range []*string{&s.DayScene, &s.NightScene} {
		if v, ok := legacy[*id]; ok {
			*id = v
		}
	}
	return s
}

func (s *settings) save() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	_ = os.WriteFile(s.path, data, 0o644)
}
