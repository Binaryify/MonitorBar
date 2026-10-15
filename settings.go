package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// monitorSettings are one display's picture choices, keyed by Monitor.ID.
type monitorSettings struct {
	SoftBrightness float64 `json:"softBrightness"`
	Warmth         float64 `json:"warmth"`
	NetworkHost    string  `json:"networkHost,omitempty"`
}

// settings are the choices kept between launches, in a JSON file.
// SoftBrightness, Warmth and NetworkHost mirror the selected display, so a
// settings file from before per-display settings still loads.
type settings struct {
	SoftBrightness float64 `json:"softBrightness"` // gamma %, 20...100
	Warmth         float64 `json:"warmth"`         // 0...100
	Tab            string  `json:"tab"`
	NetworkHost    string  `json:"networkHost"` // the display found last time
	LaunchedBefore bool    `json:"launchedBefore"`

	Selected string                     `json:"selected,omitempty"`
	Monitors map[string]monitorSettings `json:"monitors,omitempty"`

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
	path          string
}

// hostFor is the LAN address remembered for a display.
func (p *settings) hostFor(id string) string {
	if p == nil || p.Monitors == nil {
		return ""
	}
	return p.Monitors[id].NetworkHost
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
	s := &settings{SoftBrightness: 100, DayStart: "07:00", NightStart: "19:00", DayScene: "日间", NightScene: "夜间",
		FollowMin: 10, FollowMax: 100,
		path: filepath.Join(dataDir(), "settings.json")}
	if data, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(data, s)
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
