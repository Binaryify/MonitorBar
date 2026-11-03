package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are the choices kept between launches, in a JSON file.
type settings struct {
	ScreenBindings  map[string]string        `json:"screenBindings,omitempty"`
	SyncDelta       bool                     `json:"syncDelta"`
	ExcludedAudio   map[string]bool          `json:"excludedAudio,omitempty"`
	ExcludedScreens map[string]bool          `json:"excludedScreens,omitempty"`
	Pictures        map[string]pictureState  `json:"pictures,omitempty"`
	Presets         map[string]displayPreset `json:"presets,omitempty"`

	DeviceMACs     map[string]string `json:"deviceMACs,omitempty"`
	KnownHosts     []string          `json:"knownHosts,omitempty"`
	AudioBindings  map[string]string `json:"audioBindings,omitempty"`
	DeviceNames    map[string]string `json:"deviceNames,omitempty"`
	SyncVolume     bool              `json:"syncVolume"`
	SyncPicture    bool              `json:"syncPicture"`
	SyncResolution bool              `json:"syncResolution"`
	SoftBrightness float64           `json:"softBrightness"` // gamma %, 20...100
	Warmth         float64           `json:"warmth"`         // 0...100
	Tab            string            `json:"tab"`
	NetworkHost    string            `json:"networkHost"` // the display found last time
	LaunchedBefore bool              `json:"launchedBefore"`

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
	// Taking the keyboard's brightness and volume keys (macOS).
	MediaKeys  bool `json:"mediaKeys"`
	VolumeStep int  `json:"volumeStep"`
	// Showing the level under the menu bar icon as the keys change it.
	HUD bool `json:"hud"`
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
		FollowMin: 10, FollowMax: 100, HUD: true, VolumeStep: 2,
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

// moveDevice preserves every IP-keyed setting when an address changes.
func (s *settings) moveDevice(old, next string) {
	if old == next {
		return
	}
	moveMapKey(s.DeviceMACs, old, next)
	moveMapKey(s.DeviceNames, old, next)
	moveMapKey(s.AudioBindings, old, next)
	moveMapKey(s.ScreenBindings, old, next)
	moveMapKey(s.ExcludedAudio, old, next)
	for name, p := range s.Presets {
		moveMapKey(p.Volumes, old, next)
		s.Presets[name] = p
	}
	for i, h := range s.KnownHosts {
		if h == old {
			s.KnownHosts[i] = next
		}
	}
	if s.NetworkHost == old {
		s.NetworkHost = next
	}
}
func moveMapKey[V any](values map[string]V, old, next string) {
	if v, ok := values[old]; ok {
		values[next] = v
		delete(values, old)
	}
}

// volumeStep also handles older settings and invalid saved values.
func (s *settings) volumeStep() float64 {
	if s.VolumeStep < 1 || s.VolumeStep > 10 {
		return 2
	}
	return float64(s.VolumeStep)
}
