package main

import (
	"github.com/binaryify/MonitorBar/internal/mitv"
	"testing"
)

func TestBoundAudioRouting(t *testing.T) {
	s := &store{prefs: &settings{SyncVolume: true, AudioBindings: map[string]string{"left": "uid-left", "right": "uid-right"}}, link: mitv.New("left"), hasVolume: true, soundToDisplay: func() bool { return true }, activeAudioUIDs: func() []string { return []string{"uid-right"} }}
	right := &store{link: mitv.New("right"), hasVolume: true}
	s.peers = []*store{right}
	targets := s.audioKeyTargets()
	if len(targets) != 1 || targets[0] != right {
		t.Fatal("inactive output received keys")
	}
	right.offline = true
	if len(s.audioKeyTargets()) != 0 {
		t.Fatal("offline device received keys")
	}
}
