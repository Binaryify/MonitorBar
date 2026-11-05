package display

import "testing"

func TestActiveAudioRoute(t *testing.T) {
	types := map[uint32]string{1: "grup", 2: "grup", 3: "bltn", 4: "dprt", 5: "hdmi"}
	members := map[uint32][]uint32{1: {3, 2}, 2: {1, 4}}
	route := func(id uint32) AudioRoute {
		return classifyAudioRoute(id, func(id uint32) uint32 { return fourCC(types[id]) }, func(id uint32) []uint32 { return members[id] })
	}
	if route(1) != GroupDisplayAudio || route(5) != DisplayAudio {
		t.Fatal("display route missed")
	}
	members[2] = []uint32{1}
	if route(1) != OtherAudio {
		t.Fatal("disconnected display still matched")
	}
}
