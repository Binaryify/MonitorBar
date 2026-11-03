package main

import (
	"reflect"
	"testing"
)

func TestAddressMoves(t *testing.T) {
	for _, tt := range []struct {
		name               string
		saved, found, want map[string]string
	}{
		{"move", map[string]string{"a": "left"}, map[string]string{"b": "left"}, map[string]string{"a": "b"}},
		{"swap", map[string]string{"a": "left", "b": "right"}, map[string]string{"b": "left", "a": "right"}, map[string]string{"a": "b", "b": "a"}},
		{"offline collision", map[string]string{"a": "left", "b": "right"}, map[string]string{"b": "left"}, map[string]string{}},
		{"duplicate", map[string]string{"a": "left"}, map[string]string{"b": "left", "c": "left"}, map[string]string{}},
		{"missing", map[string]string{"a": "left"}, map[string]string{"b": ""}, map[string]string{}},
		{"legacy collision", map[string]string{"a": "left", "b": ""}, map[string]string{"b": "left"}, map[string]string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := addressMoves(tt.saved, tt.found); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
func TestRemapDevicesSwap(t *testing.T) {
	s := settings{DeviceMACs: map[string]string{"a": "left", "b": "right"}, DeviceNames: map[string]string{"a": "Left", "b": "Right"}, AudioBindings: map[string]string{"a": "audio-left", "b": "audio-right"}, ScreenBindings: map[string]string{"a": "screen-left", "b": "screen-right"}, ExcludedAudio: map[string]bool{"a": true}, KnownHosts: []string{"a", "b"}, NetworkHost: "a", Presets: map[string]displayPreset{"test": {Volumes: map[string]float64{"a": 20, "b": 30}}}}
	s.remapDevices(map[string]string{"a": "b", "b": "a"})
	if s.DeviceMACs["b"] != "left" || s.DeviceNames["a"] != "Right" || s.AudioBindings["b"] != "audio-left" || s.ScreenBindings["a"] != "screen-right" || !s.ExcludedAudio["b"] || s.ExcludedAudio["a"] || s.NetworkHost != "b" || s.KnownHosts[0] != "b" || s.Presets["test"].Volumes["a"] != 30 {
		t.Fatalf("settings lost in swap: %+v", s)
	}
	s.remapDevices(map[string]string{"a": "c", "b": "d"})
	if s.DeviceNames["d"] != "Left" || s.Presets["test"].Volumes["c"] != 30 {
		t.Fatal("second move lost settings")
	}
}
