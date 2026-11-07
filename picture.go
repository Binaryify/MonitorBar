package main

import "math"

func (s *store) picture() *pictureSnapshot {
	p := &pictureSnapshot{Soft: s.soft, Warmth: s.warmth, Controls: map[byte]float64{}}
	if s.hasBrightness {
		p.Backlight = new(s.brightness)
	}
	for _, c := range s.controls {
		if c.max > 0 {
			p.Controls[c.code] = c.value * 100 / float64(c.max)
		}
	}
	return p
}

func (s *store) rememberSoftware() {
	if s.beforeScene == nil {
		s.beforeScene = &pictureSnapshot{Soft: s.soft, Warmth: s.warmth}
	}
}

// Only a full read with at least one picture control establishes the baseline.
// It is persisted immediately, before any scene can mutate the newly read state.
func (s *store) captureBaseline(r ddcReading) {
	d := s.prefs.display(s.monitor.Key)
	if d.Baseline != nil || r.refreshed || (!s.hasBrightness && len(s.controls) == 0) {
		return
	}
	p := s.picture()
	if s.beforeScene != nil {
		p.Soft, p.Warmth = s.beforeScene.Soft, s.beforeScene.Warmth
	}
	d.Baseline = p
	s.dirty = true
	if s.prefs.path != "" {
		s.prefs.save()
	}
}

func validPictureValue(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100
}

func (s *store) restoreControls(p *pictureSnapshot) {
	if p == nil {
		return
	}
	for i := range s.controls {
		c := &s.controls[i]
		if v, ok := p.Controls[c.code]; ok && validPictureValue(v) && c.max > 0 {
			target := math.Round(v * float64(c.max) / 100)
			if c.value == target {
				continue
			}
			c.value = target
			s.setControl(i)
		}
	}
}

func (s *store) restorePicture(p *pictureSnapshot) {
	if p == nil {
		return
	}
	s.restoreControls(p)
	if s.hasBrightness && p.Backlight != nil && validPictureValue(*p.Backlight) && s.brightness != *p.Backlight {
		s.setBrightness(*p.Backlight)
	}
	if validPictureValue(p.Soft) && p.Soft >= softMin {
		s.soft = p.Soft
	}
	if validPictureValue(p.Warmth) {
		s.warmth = p.Warmth
	}
	s.applyGamma()
}

// The caller uses each, just like applying a scene: reset each display to
// its own baseline, rather than copying the selected display's defaults.
func (s *store) restoreDefaults() { s.restorePicture(s.prefs.display(s.monitor.Key).Baseline) }

func (s *store) matchesControls(p *pictureSnapshot) bool {
	if p == nil {
		return true
	}
	for _, c := range s.controls {
		if v, ok := p.Controls[c.code]; ok && c.max > 0 && validPictureValue(v) {
			if math.Abs(c.value-math.Round(v*float64(c.max)/100)) > 0.01 {
				return false
			}
		}
	}
	return true
}

func (s *store) matchesPicture(p *pictureSnapshot) bool {
	return math.Abs(s.soft-p.Soft) < 1 && math.Abs(s.warmth-p.Warmth) < 1 &&
		(!s.hasBrightness || p.Backlight == nil || math.Abs(s.brightness-*p.Backlight) < 1) && s.matchesControls(p)
}
