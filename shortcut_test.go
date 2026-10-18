package main

import (
	"testing"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
)

// The shortcut puts the picture back on the input this Mac is plugged into.
func TestSwitchHome(t *testing.T) {
	s := &store{prefs: &settings{}, redraw: func() {}}
	s.ddcInputs = []mitv.Source{{ID: 0x0F, Name: "DP"}, {ID: 0x11, Name: "HDMI 1"}}
	s.input, s.homeInput = 0x11, 0x0F // away on HDMI 1, this Mac is on DP
	s.switchHome()
	if s.input != 0x0F {
		t.Errorf("input after the shortcut: %#x, want 0x0f", s.input)
	}
}

// Switching inputs with no home input known does nothing.
func TestSwitchHomeWithoutHome(t *testing.T) {
	s := &store{prefs: &settings{}, redraw: func() {}}
	s.ddcInputs = []mitv.Source{{ID: 0x0F, Name: "DP"}}
	s.input = 0x0F
	s.switchHome()
	if s.input != 0x0F || s.homeInput != 0 {
		t.Errorf("input %#x home %#x", s.input, s.homeInput)
	}
}

// The monitor teaches the home input once: later switches through the app keep
// the first answer, so the shortcut always leads back to this Mac.
func TestLearnHomeInput(t *testing.T) {
	s := &store{prefs: &settings{}}
	src := func(code int) []mitv.Source { return []mitv.Source{{ID: code}} }
	s.learnHomeInput(ddcReading{values: map[byte][2]int{display.VCPInput: {0x0F, 3}}, inputs: src(0x0F)})
	if s.homeInput != 0x0F || s.prefs.HomeInput != 0x0F || !s.dirty {
		t.Fatalf("home %#x prefs %#x dirty %v", s.homeInput, s.prefs.HomeInput, s.dirty)
	}
	s.dirty = false
	s.learnHomeInput(ddcReading{values: map[byte][2]int{display.VCPInput: {0x11, 3}}, inputs: src(0x11)})
	if s.homeInput != 0x0F || s.dirty {
		t.Errorf("the home input was overwritten: %#x", s.homeInput)
	}
}

// A switch the app makes before the monitor answered still teaches it the input
// the Mac was on.
func TestSelectInputLearnsHome(t *testing.T) {
	s := &store{prefs: &settings{}, redraw: func() {}}
	s.ddcInputs = []mitv.Source{{ID: 0x0F, Name: "DP"}, {ID: 0x11, Name: "HDMI 1"}}
	s.input = 0x0F
	s.selectInput(s.ddcInputs[1])
	if s.homeInput != 0x0F || s.input != 0x11 {
		t.Errorf("home %#x input %#x", s.homeInput, s.input)
	}
}
