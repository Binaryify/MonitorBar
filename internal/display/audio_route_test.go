package display

import "testing"

func TestOutputContainsDisplay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		types   map[uint32]string
		members map[uint32][]uint32
		want    bool
	}{
		{"HDMI", map[uint32]string{1: "hdmi"}, nil, true},
		{"headphones", map[uint32]string{1: "usb"}, nil, false},
		{"multi-output", map[uint32]string{1: "grup", 2: "usb", 3: "dprt"}, map[uint32][]uint32{1: {2, 3}}, true},
		{"nested", map[uint32]string{1: "grup", 2: "grup", 3: "hdmi"}, map[uint32][]uint32{1: {2}, 2: {3}}, true},
		{"cycle", map[uint32]string{1: "grup", 2: "grup"}, map[uint32][]uint32{1: {2}, 2: {1}}, false},
		{"empty", map[uint32]string{1: "grup"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := outputContainsDisplay(1, func(id uint32) (string, []uint32) { return tc.types[id], tc.members[id] })
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
