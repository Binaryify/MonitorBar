package main

import "testing"

// Saving keeps the current picture as a scene that applies it again, and
// shows in the panel until deleted (#13).
func TestSavedScene(t *testing.T) {
	p, gamma := testPanel(t)
	s := p.s
	s.prefs.SoftBrightness, s.prefs.Warmth = 45, 35
	tt := newTester(p)
	if err := tt.Click("把当前亮度和色温保存为场景"); err != nil { // the save chip, by its label
		t.Fatal(err)
	}
	tt.Frame()
	if len(s.prefs.SavedScenes) != 1 || !tt.HasText("场景 1") {
		t.Fatalf("saved %+v; texts %q", s.prefs.SavedScenes, tt.Texts())
	}
	saved := s.sceneByID("saved-1")
	if !s.isActive(saved) {
		t.Error("the scene just saved is not highlighted")
	}

	if err := tt.Click("日间"); err != nil {
		t.Fatal(err)
	}
	if s.isActive(saved) {
		t.Error("the saved scene stays highlighted after another")
	}
	if err := tt.Click("场景 1"); err != nil {
		t.Fatal(err)
	}
	if s.prefs.SoftBrightness != 45 || s.prefs.Warmth != 35 || *gamma != [2]float64{0.45, 0.35} {
		t.Errorf("applied: brightness %v warmth %v gamma %v", s.prefs.SoftBrightness, s.prefs.Warmth, *gamma)
	}

	// The next one takes the next free number; deleting one switches by the
	// time to the built-in scene again.
	s.saveScene()
	s.prefs.NightScene = "saved-2"
	if err := tt.Click("画面"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("我的场景") {
		t.Fatalf("no card to manage the saved scenes in %q", tt.Texts())
	}
	if err := tt.Click("删除「场景 2」"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(s.prefs.SavedScenes) != 1 || s.prefs.SavedScenes[0].ID != "saved-1" || s.prefs.NightScene != "night" {
		t.Errorf("after deleting: %+v, night scene %q", s.prefs.SavedScenes, s.prefs.NightScene)
	}
	s.saveScene()
	if s.prefs.SavedScenes[1].ID != "saved-2" {
		t.Errorf("the next scene got %q", s.prefs.SavedScenes[1].ID)
	}
}

// On a monitor whose backlight DDC/CI reaches, a saved scene keeps the
// backlight and the software dimming both, so one below the backlight's
// minimum comes back as dark.
func TestSavedSceneKeepsBacklight(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.hasBrightness, s.brightness = true, 0
	s.prefs.SoftBrightness, s.prefs.Warmth = 40, 10
	s.saveScene()
	s.apply(scenes[2]) // movie: all backlight
	if s.brightness != 100 || s.prefs.SoftBrightness != 100 {
		t.Fatalf("movie: backlight %v software %v", s.brightness, s.prefs.SoftBrightness)
	}
	saved := s.sceneByID("saved-1")
	s.apply(saved)
	if s.brightness != 0 || s.prefs.SoftBrightness != 40 || s.prefs.Warmth != 10 || !s.isActive(saved) {
		t.Errorf("saved: backlight %v software %v warmth %v", s.brightness, s.prefs.SoftBrightness, s.prefs.Warmth)
	}
	if s.isActive(scenes[0]) {
		t.Error("the day scene is highlighted too")
	}
}

// No more chips than fit: the save chip goes once the scenes are many.
func TestSavedScenesLimit(t *testing.T) {
	p, _ := testPanel(t)
	for range maxSavedScenes + 2 {
		p.s.saveScene()
	}
	if n := len(p.s.prefs.SavedScenes); n != maxSavedScenes {
		t.Errorf("%d saved scenes, want at most %d", n, maxSavedScenes)
	}
	tt := newTester(p)
	if tt.Click("把当前亮度和色温保存为场景") == nil {
		t.Error("the save chip shows with no room left")
	}
}
