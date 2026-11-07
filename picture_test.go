package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/locales"
)

func TestRestoreDefaultsButton(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "en"} {
		t.Run(locale, func(t *testing.T) {
			previous := testLocale
			testLocale = locale
			defer func() { testLocale = previous }()
			p, gamma := testPanel(t)
			s := p.s
			s.hasBrightness, s.brightness, s.soft, s.warmth = true, 61, 84, 12
			s.controls = []control{{code: display.VCPContrast, max: 100, value: 48}}
			s.captureBaseline(ddcReading{})
			s.brightness, s.soft, s.warmth, s.controls[0].value = 90, 60, 45, 72
			tt := newTester(p)
			if err := tt.Click(locales.In(locale).RestoreDefaults()); err != nil {
				t.Fatal(err)
			}
			if s.brightness != 61 || s.soft != 84 || s.warmth != 12 || s.controls[0].value != 48 || *gamma != [2]float64{0.84, 0.12} {
				t.Fatal("reset button did not restore the complete captured picture")
			}
		})
	}
}

func TestSceneWaitsForInitialPictureRead(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.probingDDC = true
	s.saveScene()
	if len(s.prefs.SavedScenes) != 0 {
		t.Fatal("saved an incomplete picture while DDC controls were still loading")
	}
	s.probingDDC = false
	s.saveScene()
	if len(s.prefs.SavedScenes) != 1 {
		t.Fatal("scene unavailable after the initial picture read")
	}
}

func TestCompletePictureScene(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.hasBrightness, s.brightness, s.soft, s.warmth = true, 63, 74, 29
	for i, c := range controlCatalog {
		s.controls = append(s.controls, control{code: c.code, max: 200, value: float64(40 + i*10)})
	}
	s.saveScene()
	saved := s.sceneByID("saved-1")
	input, volume, mode := s.input, s.volume, s.current
	for i := range s.controls {
		s.controls[i].value = 199
	}
	s.soft, s.warmth, s.brightness = 100, 0, 100
	if s.isActive(saved) {
		t.Fatal("changed picture still active")
	}
	s.apply(saved)
	if !s.isActive(saved) || s.soft != 74 || s.warmth != 29 || s.brightness != 63 {
		t.Fatal("incomplete restore")
	}
	for i, c := range s.controls {
		if c.value != float64(40+i*10) {
			t.Fatalf("control %x = %v", c.code, c.value)
		}
	}
	s.controls[0].value++
	if s.isActive(saved) {
		t.Fatal("control edit still active")
	}
	if s.input != input || s.volume != volume || s.current != mode {
		t.Fatal("changed non-picture state")
	}
}

func TestBaselinePersistenceAndBuiltins(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.prefs.path = filepath.Join(t.TempDir(), "settings.json")
	s.hasBrightness, s.brightness, s.soft, s.warmth = true, 67, 81, 13
	s.controls = []control{{code: display.VCPContrast, max: 100, value: 42}}
	s.captureBaseline(ddcReading{refreshed: true})
	if s.prefs.display("mi").Baseline != nil {
		t.Fatal("partial refresh captured baseline")
	}
	s.captureBaseline(ddcReading{})
	data, err := os.ReadFile(s.prefs.path)
	if err != nil {
		t.Fatal(err)
	}
	var prefs settings
	if err = json.Unmarshal(data, &prefs); err != nil {
		t.Fatal(err)
	}
	s.prefs = &prefs
	s.controls[0].value = 90
	s.captureBaseline(ddcReading{})
	s.apply(scenes[0])
	if s.controls[0].value != 42 || s.brightness != 80 || s.soft != 100 || s.warmth != 0 {
		t.Fatal("built-in did not restore baseline controls with preset levels")
	}
	s.controls[0].value = 99
	if s.isActive(scenes[0]) {
		t.Fatal("built-in active with wrong controls")
	}
	s.restoreDefaults()
	if s.brightness != 67 || s.soft != 81 || s.warmth != 13 || s.controls[0].value != 42 {
		t.Fatal("baseline overwritten or incomplete")
	}
}

func TestPictureLegacyAndUnsupported(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	var legacy savedScene
	if err := json.Unmarshal([]byte(`{"id":"old","softBrightness":70,"warmth":20}`), &legacy); err != nil {
		t.Fatal(err)
	}
	s.controls = []control{{code: display.VCPContrast, max: 100, value: 32}}
	s.apply(scene{saved: &legacy, warmth: legacy.Warmth})
	if s.soft != 70 || s.warmth != 20 || s.controls[0].value != 32 {
		t.Fatal("legacy scene changed unknown controls")
	}
	s.restorePicture(&pictureSnapshot{Soft: 80, Warmth: 10, Controls: map[byte]float64{display.VCPVolume: 0, display.VCPContrast: 150}})
	if s.controls[0].value != 32 {
		t.Fatal("invalid value applied")
	}
	s.controls[0].max = 200
	s.restorePicture(&pictureSnapshot{Soft: 80, Warmth: 10, Controls: map[byte]float64{display.VCPContrast: 25}})
	if s.controls[0].value != 50 {
		t.Fatal("range not scaled")
	}
}

func TestBaselineSyncAndNewDisplay(t *testing.T) {
	p, _ := twoDisplays(t)
	s := p.s
	s.every(func() { s.captureBaseline(ddcReading{}) })
	if s.prefs.display("dell").Baseline != nil {
		t.Fatal("invented DDC baseline")
	}
	s.hasBrightness = true
	s.brightness = 60
	s.soft = 77
	s.captureBaseline(ddcReading{})
	s.soft = 100
	s.prefs.SyncDisplays = true
	s.each(s.restoreDefaults)
	if s.soft != 77 || s.outputs[1].soft != 100 {
		t.Fatal("reset did not use per-display baseline")
	}
	s.rememberSoftware()
	s.soft = 30
	if s.prefs.display("mi").Baseline.Soft != 77 {
		t.Fatal("baseline aliased live state")
	}
}

func TestFullDDCReadCapturesBaselineOnce(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.soft, s.warmth = 76, 18
	// A scene can be selected before the initial asynchronous probe returns.
	s.apply(scenes[3])
	s.ddc = &display.DDC{} // applyDDC only consumes the supplied reading; no I/O
	s.applyDDC(ddcReading{values: map[byte][2]int{
		display.VCPBrightness: {120, 200}, display.VCPContrast: {37, 100},
		display.VCPVolume: {90, 100}, display.VCPInput: {15, 20},
	}})
	b := s.prefs.display("mi").Baseline
	if b == nil || b.Soft != 76 || b.Warmth != 18 || *b.Backlight != 60 || b.Controls[display.VCPContrast] != 37 {
		t.Fatalf("wrong first baseline: %+v", b)
	}
	if len(b.Controls) != 1 {
		t.Fatal("captured non-picture controls")
	}
	s.applyDDC(ddcReading{values: map[byte][2]int{
		display.VCPBrightness: {10, 100}, display.VCPContrast: {80, 100},
	}})
	if s.prefs.display("mi").Baseline != b || b.Controls[display.VCPContrast] != 37 {
		t.Fatal("probe replaced baseline")
	}
	s.ddc = nil
	s.applyDDC(ddcReading{})
	if s.hasBrightness || len(s.controls) != 0 || s.prefs.display("mi").Baseline != b {
		t.Fatal("disconnect lost baseline or kept unsupported controls")
	}
	s.restoreDefaults()
	if s.soft != 76 || s.warmth != 18 {
		t.Fatal("software baseline unavailable after disconnect")
	}
}

func TestPictureSceneJSONRoundTrip(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.hasBrightness, s.brightness, s.soft, s.warmth = true, 64, 83, 24
	s.controls = []control{{code: display.VCPRedGain, max: 255, value: 153}}
	s.saveScene()
	data, err := json.Marshal(s.prefs)
	if err != nil {
		t.Fatal(err)
	}
	var prefs settings
	if err := json.Unmarshal(data, &prefs); err != nil {
		t.Fatal(err)
	}
	s.prefs = &prefs
	s.controls[0].value = 0
	s.brightness, s.soft, s.warmth = 0, 100, 0
	s.apply(s.sceneByID("saved-1"))
	if s.controls[0].value != 153 || s.brightness != 64 || s.soft != 83 || s.warmth != 24 {
		t.Fatal("scene lost picture after restart")
	}
}
