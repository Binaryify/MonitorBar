package main

import (
	"path/filepath"
	"testing"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
)

// twoDisplays is the test panel with a second display, a Dell reached by
// neither the LAN nor DDC/CI, and records the gamma each display was given.
func twoDisplays(t *testing.T) (*panel, map[string][2]float64) {
	t.Helper()
	p, _ := testPanel(t)
	s := p.s
	gammas := map[string][2]float64{}
	s.setGamma = func(b, w float64) error { gammas[s.monitor.Key] = [2]float64{b, w}; return nil }
	dell := &output{monitor: display.Monitor{Key: "dell", Name: "DELL U2723QE"}, soft: 100, lastBuiltin: -1}
	s.outputs = append(s.outputs, dell)
	return p, gammas
}

// Each display keeps its own picture; the switcher shows the one chosen.
func TestDisplaysApart(t *testing.T) {
	p, gammas := twoDisplays(t)
	s := p.s
	tt := newTester(p)
	if !tt.HasText("DELL U2723QE") || !tt.HasText("同步") {
		t.Fatalf("no switcher in %q", tt.Texts())
	}
	if err := tt.Click("DELL U2723QE"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.monitor.Key != "dell" || s.prefs.Display != "dell" {
		t.Fatalf("shows %q, kept %q", s.monitor.Key, s.prefs.Display)
	}
	if tt.HasText("HDMI1") || tt.HasText("显示器音量") {
		t.Error("the Dell shows the inputs and volume of the Mi Monitor")
	}
	if err := tt.Click("夜间"); err != nil {
		t.Fatal(err)
	}
	if gammas["dell"] != [2]float64{0.3, 0.55} {
		t.Errorf("the Dell's gamma %v", gammas["dell"])
	}
	if _, touched := gammas["mi"]; touched || s.outputs[0].soft != 100 {
		t.Errorf("the scene reached the Mi Monitor: gamma %v, brightness %v", gammas["mi"], s.outputs[0].soft)
	}

	// Each is kept by its key.
	s.prefs.path = filepath.Join(t.TempDir(), "settings.json")
	s.dirty = true
	s.save()
	if d := s.prefs.Displays["dell"]; d == nil || d.SoftBrightness != 30 || d.Warmth != 55 {
		t.Errorf("the Dell's settings %+v", d)
	}
	if d := s.prefs.Displays["mi"]; d == nil || d.SoftBrightness != 100 || s.prefs.SoftBrightness != 100 {
		t.Errorf("the Mi Monitor's settings %+v, the first display's %v", d, s.prefs.SoftBrightness)
	}
}

// In sync, the picture, the volume and the input change on every display
// that has them.
func TestDisplaysInSync(t *testing.T) {
	p, gammas := twoDisplays(t)
	s := p.s
	second := &output{monitor: display.Monitor{Key: "mi2", Name: "Mi Monitor"}, soft: 100, lastBuiltin: -1,
		link: mitv.New("192.0.2.2"), inputs: s.inputs, volume: 40, hasVolume: true, deviceVolume: 40}
	s.outputs = append(s.outputs, second)
	tt := newTester(p)
	for _, want := range []string{"Mi Monitor 1", "Mi Monitor 3", "DELL U2723QE"} {
		if !tt.HasText(want) {
			t.Errorf("switcher: missing %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("同步"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("阅读"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"mi", "dell", "mi2"} {
		if gammas[key] != [2]float64{0.55, 0.25} {
			t.Errorf("%s: gamma %v, want the reading scene", key, gammas[key])
		}
	}
	if err := tt.Click("DP"); err != nil {
		t.Fatal(err)
	}
	if s.outputs[0].input != 29 || second.input != 29 || s.outputs[1].input != 0 {
		t.Errorf("inputs %d %d %d, want DP on both Mi Monitors", s.outputs[0].input, s.outputs[1].input, second.input)
	}
	s.toggleMute()
	if s.outputs[0].volume != 0 || second.volume != 0 || second.unmuted != 40 {
		t.Errorf("muted: %v and %v (back to %v)", s.outputs[0].volume, second.volume, second.unmuted)
	}
	s.toggleMute()
	if s.outputs[0].volume != 80 || second.volume != 40 {
		t.Errorf("unmuted: %v and %v", s.outputs[0].volume, second.volume)
	}
}

// The monitors found on the LAN pair as last time, then in turn, Xiaomi's
// displays first; choosing one for a display gives the other display the
// one it had.
func TestPairLinks(t *testing.T) {
	p, _ := twoDisplays(t)
	s := p.s
	dell, mi := s.outputs[1], s.outputs[0]
	s.outputs = []*output{dell, mi} // the Dell first, as macOS may list them
	mi.link, mi.inputs = nil, nil
	second := &output{monitor: display.Monitor{Key: "mi2", Name: "Redmi 27"}, soft: 100, lastBuiltin: -1}
	s.outputs = append(s.outputs, second)
	lm := func(host string) lanMonitor {
		return lanMonitor{link: mitv.New(host), name: "Xiaomi 显示器", sources: []mitv.Source{{ID: 29, Name: "DP"}}, volume: 10, hasVolume: true}
	}
	s.lan = []lanMonitor{lm("192.0.2.10"), lm("192.0.2.11")}
	s.prefs.display("mi2").NetworkHost = "192.0.2.11"
	s.pairLinks()
	host := func(o *output) string {
		if o.link == nil {
			return ""
		}
		return o.link.Host
	}
	if host(dell) != "" || host(mi) != "192.0.2.10" || host(second) != "192.0.2.11" {
		t.Fatalf("paired: dell %q, mi %q, redmi %q", host(dell), host(mi), host(second))
	}
	if !mi.hasVolume || mi.volume != 10 || len(mi.inputs) != 1 {
		t.Errorf("the Mi Monitor's remote: volume %v %v, inputs %v", mi.hasVolume, mi.volume, mi.inputs)
	}

	s.showDisplay(mi)
	s.chooseLink("192.0.2.11")
	if host(mi) != "192.0.2.11" || host(second) != "192.0.2.10" {
		t.Errorf("after choosing: mi %q, redmi %q; want them swapped", host(mi), host(second))
	}
	s.chooseLink("")
	if host(mi) != "" || host(second) != "192.0.2.10" || s.prefs.display("mi").NetworkHost != noLink {
		t.Errorf("after choosing none: mi %q, redmi %q, kept %q", host(mi), host(second), s.prefs.display("mi").NetworkHost)
	}
	s.pairLinks() // as at the next launch: the choice of none stays
	if host(mi) != "" || host(second) != "192.0.2.10" {
		t.Errorf("paired again: mi %q, redmi %q", host(mi), host(second))
	}
}

// A display seen for the first time starts from the settings of the one
// MonitorBar managed before, and the first keeps its LAN remote.
func TestFirstDisplayKeepsOldSettings(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.prefs.SoftBrightness, s.prefs.Warmth, s.prefs.NetworkHost = 60, 20, "192.0.2.1"
	if d := s.prefs.display("new"); d.SoftBrightness != 60 || d.Warmth != 20 {
		t.Errorf("a new display's settings %+v", d)
	}
	if s.savedHost(s.outputs[0]) != "192.0.2.1" {
		t.Errorf("the first display's host %q", s.savedHost(s.outputs[0]))
	}
}
