package main

import (
	"encoding/json"
	"testing"
)

func TestLocalPreferencesRetainBaseline50Values(t *testing.T) {
	// Synthetic preferences only: never load the real settings file.
	raw := []byte(`{"futurePictureDefaults":{"16":50},"displays":{"test-external-display":{"futurePictureDefaults":{"18":50},"baseline":{"softBrightness":100,"warmth":20,"controls":{"18":50}},"softBrightness":75,"warmth":20}}}`)
	var s settings
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s.InputMirroring || s.display("test-external-display").InputMirroring {
		t.Fatal("enabled by default")
	}
	s.InputMirroring = true
	s.InputMirroring = false
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	json.Unmarshal(data, &got)
	if string(got["futurePictureDefaults"]) != `{"16":50}` {
		t.Fatalf("lost top level baseline: %s", data)
	}
	var displays map[string]map[string]json.RawMessage
	json.Unmarshal(got["displays"], &displays)
	if string(displays["test-external-display"]["futurePictureDefaults"]) != `{"18":50}` {
		t.Fatalf("lost display baseline: %s", data)
	}
	if s.display("test-external-display").Baseline.Controls[18] != 50 {
		t.Fatal("scene baseline changed")
	}
	if d := s.display("test-external-display"); d.macInput() != 0x11 || d.winInput() != 0x12 {
		t.Fatal("incorrect wiring defaults")
	}
}
