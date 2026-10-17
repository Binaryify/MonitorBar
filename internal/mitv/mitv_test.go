package mitv

import (
	"slices"
	"testing"
)

func TestNamedMonitor(t *testing.T) {
	for name, want := range map[string]bool{
		"binaryify的REDMI 显示器 G Pro 27U 2026": true,
		"Xiaomi Monitor":         true,
		"厉害的REDMI G Pro 27U":     false, // renamed: told by its inputs (#5)
		"庄煜的Redmi 智能电视 L65MA-XT": false,
		"小米电视":                   false,
		"":                       false,
	} {
		if got := namedMonitor(name); got != want {
			t.Errorf("namedMonitor(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestHasMonitorInput(t *testing.T) {
	hdmi := []Source{{23, "HDMI1"}, {24, "HDMI2"}}
	for _, c := range []struct {
		sources []Source
		want    bool
	}{
		{append(hdmi, Source{29, "DP"}), true},
		{append(hdmi, Source{30, "USB-C"}), true},
		{append(hdmi, Source{25, "HDMI3"}), false}, // a TV
		{nil, false},
	} {
		if got := hasMonitorInput(c.sources); got != c.want {
			t.Errorf("hasMonitorInput(%v) = %v, want %v", c.sources, got, c.want)
		}
	}
}

func TestCompetitiveSteps(t *testing.T) {
	for _, on := range []bool{true, false} {
		var keys []string
		for _, s := range competitiveSteps(on) {
			keys = append(keys, s.key)
		}
		set := "left"
		if on {
			set = "right"
		}
		want := []string{"menu", "right", "right", "right", "right", "right", "enter", set}
		if !slices.Equal(keys, want) {
			t.Errorf("competitiveSteps(%v) = %v, want %v", on, keys, want)
		}
	}
}
