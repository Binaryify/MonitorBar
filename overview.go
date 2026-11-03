package main

import (
	"slices"
	"strings"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/locales"
)

// overviewVolumes represents each sync group once; excluded devices stay
// independent. Keep offline members visible without hiding the whole group.
func (s *store) overviewVolumes() [][]*store {
	var groups [][]*store
	var synced []*store
	for _, d := range append([]*store{s}, s.peers...) {
		if !d.hasVolume {
			continue
		}
		if s.prefs.SyncVolume && !s.excludedAudio(d) {
			synced = append(synced, d)
		} else {
			groups = append(groups, []*store{d})
		}
	}
	if len(synced) > 0 {
		// Use an online member as the slider's reference, when available.
		for i, d := range synced {
			if !d.offline {
				synced[0], synced[i] = synced[i], synced[0]
				break
			}
		}
		groups = append([][]*store{synced}, groups...)
	}
	return groups
}
func (s *store) deviceLabel() string {
	if s.link != nil {
		if name := strings.TrimSpace(s.prefs.DeviceNames[s.link.Host]); name != "" {
			return name
		}
		return s.link.Host
	}
	return s.screenName(s.monitor.Key(), s.monitor.Name)
}
func volumeGroupNames(devices []*store) string {
	names := make([]string, 0, len(devices))
	for _, d := range devices {
		names = append(names, d.deviceLabel())
	}
	return strings.Join(names, " + ")
}
func (s *store) screenName(key, fallback string) string {
	// Ambiguous bindings must not assign a random alias by map iteration order.
	var names []string
	for host, bound := range s.prefs.ScreenBindings {
		if bound == key {
			if name := strings.TrimSpace(s.prefs.DeviceNames[host]); name != "" {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	if len(names) > 0 {
		return strings.Join(names, " + ")
	}
	return or(fallback, locales.MonitorUnnamed())
}
func (s *store) overviewTitle() string {
	for _, group := range s.overviewVolumes() {
		if len(group) > 1 {
			return volumeGroupNames(group)
		}
	}
	return s.screenName(s.monitor.Key(), s.monitor.Name)
}

func (s *store) selectScreen(m display.Monitor) {
	s.monitor = m
	if picture, ok := s.prefs.Pictures[m.Key()]; ok {
		s.prefs.SoftBrightness, s.prefs.Warmth = picture.Brightness, picture.Warmth
	}
	s.modes = m.Modes()
	s.current, s.hasCurrent = m.CurrentMode()
	s.probeDDC()
}
