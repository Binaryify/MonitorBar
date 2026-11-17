package main

import (
	"github.com/binaryify/MonitorBar/internal/display"
	"testing"
)

type storeMirrorLayout struct{ value int }

func (*storeMirrorLayout) Release() {}

type storeMirrorBackend struct{ state int }

func (b *storeMirrorBackend) Snapshot() (display.MirrorLayout, error) {
	return &storeMirrorLayout{b.state}, nil
}
func (b *storeMirrorBackend) Matches(l display.MirrorLayout) bool {
	return b.state == l.(*storeMirrorLayout).value
}
func (b *storeMirrorBackend) Mirror() error { b.state = 1; return nil }
func (b *storeMirrorBackend) Restore(l display.MirrorLayout) error {
	b.state = l.(*storeMirrorLayout).value
	return nil
}
func TestMirroringSuspendsGammaFollowAndProtectsFallback(t *testing.T) {
	p, gamma := testPanel(t)
	s := p.s
	o := s.output
	b := &storeMirrorBackend{}
	o.mirror = &display.InputMirror{Backend: b, Mac: 0x11, Windows: 0x12}
	if err := o.mirror.Arm(); err != nil {
		t.Fatal(err)
	}
	s.soft, s.warmth = 74, 32
	s.prefs.FollowBuiltin = true
	s.hasBuiltin = true
	s.hasBrightness = true
	s.easing = true
	s.applyGamma()
	reads := 0
	s.followTick(func() (float64, bool) { reads++; return .9, true })
	if reads != 0 || *gamma != [2]float64{} || s.soft != 74 || s.warmth != 32 {
		t.Fatal("mirror altered picture or followed built-in")
	}
	s.outputs = []*output{o}
	for _, fallback := range []display.Monitor{{Builtin: true, Key: "builtin"}, {}} {
		s.output = &output{monitor: fallback, soft: 50, warmth: 50}
		s.applyGamma()
		if *gamma != [2]float64{} {
			t.Fatal("gamma reached fallback built-in")
		}
	}
	s.output = o
	if err := o.mirror.Stop(); err != nil {
		t.Fatal(err)
	}
	s.finishMirror(nil)
	if *gamma != [2]float64{.74, .32} {
		t.Fatalf("picture not restored: %v", *gamma)
	}
}
func TestManualBacklightCancelsActiveGlide(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.prefs.FollowBuiltin = true
	s.hasBuiltin = true
	s.hasBrightness = true
	s.lastBuiltin = .5
	s.followTarget = 90
	s.easing = true
	s.setBrightness(42)
	s.followTick(func() (float64, bool) { return .5, true })
	if s.brightness != 42 || s.easing {
		t.Fatal("manual edit was overwritten")
	}
	s.followTick(func() (float64, bool) { return .6, true })
	if s.brightness == 42 {
		t.Fatal("new reading did not resume following")
	}
}
