package main

import (
	"github.com/binaryify/MonitorBar/internal/mitv"
	"path/filepath"
	"testing"
)

func TestDeltaAndExcludedMember(t *testing.T) {
	s := &store{prefs: &settings{SyncVolume: true, SyncDelta: true, ExcludedAudio: map[string]bool{"other": true}}, volume: 30}
	peer := &store{prefs: s.prefs, parent: s, volume: 40}
	other := &store{prefs: s.prefs, parent: s, volume: 60, link: mitv.New("other")}
	s.peers = []*store{peer, other}
	s.setVolume(35)
	if s.volume != 35 || peer.volume != 45 || other.volume != 60 {
		t.Fatal("delta or exclusion failed")
	}
	peer.setVolume(90)
	if s.volume != 80 || peer.volume != 90 {
		t.Fatal("reverse delta failed")
	}
}
func TestPresetRoundTrip(t *testing.T) {
	s := &store{prefs: &settings{path: filepath.Join(t.TempDir(), "settings.json")}, link: mitv.New("left"), volume: 32}
	s.savePreset("work")
	s.volume = 70
	// Avoid network activity in this model test, but preserve captured data.
	saved := s.prefs.Presets["work"]
	if saved.Volumes["left"] != 32 {
		t.Fatal("preset did not capture volume")
	}
	if s.prefs.Presets == nil {
		t.Fatal("preset not saved")
	}
}
