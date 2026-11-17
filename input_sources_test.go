package main

import (
	"github.com/binaryify/MonitorBar/internal/mitv"
	"testing"
)

func TestConfiguredInputsSurviveMissingCapabilities(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil
	s.ddcInputs = nil
	cfg := s.prefs.display(s.monitor.Key)
	cfg.InputMirroring = true
	sources := s.sources()
	if len(sources) != 2 || sources[0].ID != 17 || sources[1].ID != 18 {
		t.Fatalf("configured inputs disappeared: %v", sources)
	}
	tt := newTester(p)
	if !tt.HasText("HDMI 1") || !tt.HasText("HDMI 2") {
		t.Fatal("fixed HDMI buttons missing")
	}
}
func TestRecordedInputShortcutSurvivesMissingCapabilities(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil
	s.ddcInputs = nil
	s.prefs.display(s.monitor.Key).InputMirroring = true
	s.prefs.Shortcuts = map[string]string{"input-17": "Ctrl+Alt+1", "input-18": "Ctrl+Alt+2", "input-999": "Ctrl+Alt+3", "input-15": ""}
	if len(s.sources()) != 2 {
		t.Fatalf("wrong fallback sources: %v", s.sources())
	}
	for _, sc := range s.boundShortcuts() {
		if sc.id == "input-18" {
			sc.run(s)
		}
	}
	if s.input != 18 {
		t.Fatal("recorded shortcut was ignored after capability query failed")
	}
	s.prefs.display(s.monitor.Key).InputMirroring = false
	s.ddcInputs = []mitv.Source{{ID: 15, Name: "DP"}}
	if len(s.sources()) != 1 || s.sources()[0].ID != 15 {
		t.Fatal("fallback overwrote a valid capability list")
	}
}

func TestMirrorSwitchControlsVisibleWithoutCapabilityList(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil
	s.ddcInputs = nil
	s.prefs.Tab = "picture"
	s.prefs.InputMirroring = true
	s.prefs.display(s.monitor.Key).InputMirroring = true
	tt := newTester(p)
	for _, label := range []string{"切回 Mac", "切到 Windows", "恢复扩展布局"} {
		if !tt.HasText(label) {
			t.Fatalf("missing control %q, actual %q", label, tt.Texts())
		}
	}
}
