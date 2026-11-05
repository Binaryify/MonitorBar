//go:build darwin

package display

import "testing"

func TestPickAV(t *testing.T) {
	redmi := func(serial uint32) avIdentity { return avIdentity{0x61a9, 0x2701, serial, "Redmi"} }
	dell := avIdentity{0x10ac, 0xd0a0, 7, "DELL"}
	for _, c := range []struct {
		name  string
		ids   []avIdentity
		want  avIdentity
		order int
		pick  int
	}{
		{"none", nil, dell, 0, -1},
		{"the only one, first display", []avIdentity{dell}, redmi(0), 0, 0},
		{"the only one is another model's", []avIdentity{dell}, redmi(0), 1, -1},
		{"by model", []avIdentity{dell, redmi(0)}, redmi(0), 0, 1},
		{"by serial", []avIdentity{redmi(1), redmi(2)}, redmi(2), 0, 1},
		{"identical, by place", []avIdentity{redmi(0), redmi(0)}, redmi(0), 1, 1},
		{"no identities, by place", []avIdentity{{}, {}}, redmi(0), 1, 1},
		{"no match", []avIdentity{dell, dell}, redmi(0), 0, -1},
	} {
		if got := pickAV(c.ids, c.want, c.order); got != c.pick {
			t.Errorf("%s: %d, want %d", c.name, got, c.pick)
		}
	}
}
