package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are the choices kept between launches, in a JSON file.
type settings struct {
	// The first display's picture and LAN remote, as before each display
	// had its own: the defaults of a display seen for the first time.
	SoftBrightness float64 `json:"softBrightness"` // gamma %, 20...100
	Warmth         float64 `json:"warmth"`         // 0...100
	NetworkHost    string  `json:"networkHost"`
	Tab            string  `json:"tab"`
	LaunchedBefore bool    `json:"launchedBefore"`

	// Each display's own, by its key (#14).
	Displays map[string]*displaySettings `json:"displays,omitempty"`
	Display  string                      `json:"display,omitempty"` // the one the panel shows
	// Whether the picture, volume and input change on every display at once.
	SyncDisplays bool `json:"syncDisplays"`

	// Switching to the day and night scenes at set times.
	AutoSwitch bool   `json:"autoSwitch"`
	DayStart   string `json:"dayStart"` // "07:00"
	NightStart string `json:"nightStart"`
	DayScene   string `json:"dayScene"`
	NightScene string `json:"nightScene"`
	AutoPeriod string `json:"autoPeriod"` // the period whose scene was applied last

	// Scenes the user saved, after the built-in ones (#13).
	SavedScenes []savedScene `json:"savedScenes,omitempty"`

	// Following the built-in display's brightness over DDC/CI (MacBooks).
	FollowBuiltin bool    `json:"followBuiltin"`
	FollowMin     float64 `json:"followMin"` // the external backlight at the built-in's darkest, %
	FollowMax     float64 `json:"followMax"` // and at its brightest

	// Shortcuts the user recorded, by id: absent for the default keys, ""
	// for none.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
	// Taking the keyboard's brightness and volume keys (macOS).
	MediaKeys  bool `json:"mediaKeys"`
	VolumeStep int  `json:"volumeStep,omitempty"`
	// Showing the level under the menu bar icon as the keys change it.
	HUD bool `json:"hud"`
	// The language chosen in the panel, "" to follow the system's.
	Language string `json:"language,omitempty"`
	path     string
}

// displaySettings are a display's picture and the LAN remote paired with it.
type displaySettings struct {
	SoftBrightness float64          `json:"softBrightness"`
	Warmth         float64          `json:"warmth"`
	Baseline       *pictureSnapshot `json:"baseline,omitempty"`
	NetworkHost    string           `json:"networkHost,omitempty"` // noLink when the user chose none
}

// noLink pairs a display with no LAN remote, by the user's choice.
const noLink = "none"

// display is the settings of the display of key, made on first use from the
// defaults.
func (s *settings) display(key string) *displaySettings {
	d := s.Displays[key]
	if d == nil {
		if s.Displays == nil {
			s.Displays = map[string]*displaySettings{}
		}
		d = &displaySettings{SoftBrightness: s.SoftBrightness, Warmth: s.Warmth}
		s.Displays[key] = d
	}
	return d
}

// savedScene is the picture as it was when the user saved it.
type savedScene struct {
	Picture   *pictureSnapshot `json:"picture,omitempty"`
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Backlight *float64         `json:"backlight,omitempty"` // DDC/CI %, absent when it reached none
	Soft      float64          `json:"softBrightness"`
	Warmth    float64          `json:"warmth"`
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
		FollowMin: 10, FollowMax: 100, HUD: true,
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

func (s *settings) volumeStep() float64 {
	if s.VolumeStep < 1 || s.VolumeStep > 10 {
		return shortcutStep
	}
	return float64(s.VolumeStep)
}

// pictureSnapshot owns its values. Controls are normalized percentages so scenes
// can safely follow sync-display semantics across different hardware ranges.
// A missing entry means leave that control alone, never an inferred default.
type pictureSnapshot struct {
	Backlight *float64         `json:"backlight,omitempty"`
	Soft      float64          `json:"softBrightness"`
	Warmth    float64          `json:"warmth"`
	Controls  map[byte]float64 `json:"controls,omitempty"`
}
