package main

import (
	"fmt"
	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/locales"
	"github.com/egoist/mygo/ui"
	"slices"
	"strings"
)

type pictureState struct {
	Brightness float64 `json:"brightness"`
	Warmth     float64 `json:"warmth"`
}
type displayPreset struct {
	Volumes  map[string]float64      `json:"volumes"`
	Pictures map[string]pictureState `json:"pictures"`
	Modes    map[string]string       `json:"modes"`
}

func (s *store) excludedAudio(d *store) bool {
	return d.link != nil && s.prefs.ExcludedAudio[d.link.Host]
}
func (s *store) savePreset(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	preset := displayPreset{map[string]float64{}, map[string]pictureState{}, map[string]string{}}
	for _, d := range append([]*store{s}, s.peers...) {
		if d.link != nil && !d.offline {
			preset.Volumes[d.link.Host] = d.volume
		}
	}
	for _, m := range s.screens {
		k := m.Key()
		if k == "" {
			continue
		}
		picture, ok := s.prefs.Pictures[k]
		if !ok {
			picture = pictureState{100, 0}
		}
		if m.Same(s.monitor) {
			picture = pictureState{s.prefs.SoftBrightness, s.prefs.Warmth}
		}
		preset.Pictures[k] = picture
		if mode, ok := m.CurrentMode(); ok {
			preset.Modes[k] = mode.ID()
		}
	}
	if s.prefs.Presets == nil {
		s.prefs.Presets = map[string]displayPreset{}
	}
	s.prefs.Presets[name] = preset
	s.dirty = true
	s.save()
}
func (s *store) applyPreset(name string) {
	preset, ok := s.prefs.Presets[name]
	if !ok {
		return
	}
	type change struct {
		m    display.Monitor
		mode display.Mode
	}
	var changes []change
	for _, m := range s.screens {
		wanted := preset.Modes[m.Key()]
		if wanted == "" {
			continue
		}
		found := false
		for _, mode := range m.Modes() {
			if mode.ID() == wanted {
				changes = append(changes, change{m, mode})
				found = true
				break
			}
		}
		if !found {
			s.notice = locales.SyncModeUnsupported()
			return
		}
	}
	for _, c := range changes {
		if err := c.m.Apply(c.mode); err != nil {
			s.notice = locales.SyncModeFailed()
			return
		}
	}
	if s.prefs.Pictures == nil {
		s.prefs.Pictures = map[string]pictureState{}
	}
	for _, m := range s.screens {
		if pic, ok := preset.Pictures[m.Key()]; ok {
			if err := m.SetGamma(pic.Brightness/100, pic.Warmth/100); err != nil {
				s.notice = locales.NoticeGammaFailed()
				return
			}
			s.prefs.Pictures[m.Key()] = pic
			if m.Same(s.monitor) {
				s.prefs.SoftBrightness, s.prefs.Warmth = pic.Brightness, pic.Warmth
			}
		}
	}
	for _, d := range append([]*store{s}, s.peers...) {
		if d.link != nil {
			if v, ok := preset.Volumes[d.link.Host]; ok {
				d.setLocalVolume(v)
			}
		}
	}
	s.dirty = true
	s.save()
	s.refreshDisplay()
}
func (p *panel) presetControls(c *ui.Context) {
	s := p.s
	ui.Text(c, locales.PresetName()).FontSize(11)
	ui.TextInput(c.Key("preset-name"), &s.presetName).Label(locales.PresetName())
	if chipText(c.Key("save-preset"), locales.PresetSave(), false) {
		s.savePreset(s.presetName)
	}
	var names []string
	for name := range s.prefs.Presets {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if chipText(c.Key("remove-preset-"+name), locales.PresetDelete()+" · "+name, false) {
			delete(s.prefs.Presets, name)
			s.dirty = true
			s.save()
			continue
		}
		if chipText(c.Key("preset-"+name), name, false) {
			s.applyPreset(name)
		}
	}
}

func (s *store) screenLabel(index int, key, name string) string {
	for host, bound := range s.prefs.ScreenBindings {
		if bound == key && s.prefs.DeviceNames[host] != "" {
			name = s.prefs.DeviceNames[host]
			break
		}
	}
	return fmt.Sprintf("%d · %s", index+1, name)
}
