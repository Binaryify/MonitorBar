package main

import (
	"github.com/binaryify/MonitorBar/internal/mediakeys"
	"github.com/binaryify/MonitorBar/locales"
	"testing"
)

func TestDualVolumeRouting(t *testing.T) {
	root := &store{prefs: &settings{}, volume: 30, hasVolume: true}
	peer := &store{prefs: root.prefs, parent: root, volume: 60, hasVolume: true}
	root.peers = []*store{peer}
	peer.setVolume(45)
	if root.volume != 30 || peer.volume != 45 {
		t.Fatal("independent volume changed other device")
	}
	root.prefs.SyncVolume = true
	peer.setVolume(25)
	if root.volume != 25 || peer.volume != 25 {
		t.Fatal("sync must work from either device")
	}
	root.toggleMute()
	if root.volume != 0 || peer.volume != 0 {
		t.Fatal("mute was not synchronized")
	}
	root.toggleMute()
	if root.volume != 25 || peer.volume != 25 {
		t.Fatal("unmute did not restore level")
	}
	root.prefs.SyncVolume = false
	root.setVolume(10)
	if peer.volume != 25 {
		t.Fatal("disabled sync still changed peer")
	}
}

func TestSyncPage(t *testing.T) {
	p, _ := testPanel(t)
	p.s.prefs.Tab = "dual"
	tt := newTester(p)
	for _, label := range []string{locales.SyncVolume(), locales.SyncPicture(), locales.SyncResolution()} {
		if !tt.HasText(label) {
			t.Errorf("missing sync control %s", label)
		}
	}
}

func TestSyncedMediaKeys(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil
	s.prefs.SyncVolume = true
	peer := &store{prefs: s.prefs, parent: s, volume: 80, hasVolume: true}
	s.peers = []*store{peer}
	step, pass := s.mediaKey(mediakeys.VolumeDown)
	if step == nil || pass {
		t.Fatal("display output volume key not intercepted")
	}
	step()
	if peer.volume != s.volume || s.volume >= 80 {
		t.Fatal("volume key did not sync")
	}
	s.soundToDisplay = func() bool { return false }
	step, _ = s.mediaKey(mediakeys.VolumeUp)
	if step != nil {
		t.Fatal("headphone output intercepted")
	}
}
func TestSyncPageLanguages(t *testing.T) {
	old := testLocale
	defer func() { testLocale = old }()
	for _, lang := range []string{"en", "zh-Hans"} {
		testLocale = lang
		p, _ := testPanel(t)
		p.s.prefs.Tab = "dual"
		tt := newTester(p)
		if !tt.HasText(locales.SyncVolume()) {
			t.Fatal("missing translated switch", lang)
		}
	}
}

func TestBrightnessMediaKey(t *testing.T) {
	p, gamma := testPanel(t)
	step, _ := p.s.mediaKey(mediakeys.BrightnessDown)
	if step == nil {
		t.Fatal("external screen brightness key not handled")
	}
	step()
	if gamma[0] >= 1 || gamma[0] <= 0 {
		t.Fatal("brightness key did not dim screen", gamma)
	}
}
