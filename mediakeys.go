package main

import (
	"github.com/egoist/mygo"
	"slices"

	"github.com/binaryify/MonitorBar/internal/mediakeys"
)

// mediaKey is what a brightness or volume key does while MonitorBar takes
// the keys: step changes the external display, nil to leave the key to
// macOS, and pass lets macOS have the key as well.
func (s *store) mediaKey(k mediakeys.Key) (step func(), pass bool) {
	switch k {
	case mediakeys.BrightnessUp, mediakeys.BrightnessDown:
		if s.monitor.Builtin || s.prefs.FollowBuiltin && s.canFollow() {
			return nil, false // no external display, or it follows the built-in one already
		}
		delta := float64(shortcutStep)
		if k == mediakeys.BrightnessDown {
			delta = -delta
		}
		// A MacBook's own display keeps its keys: both change together.
		return func() { s.stepBrightness(delta); s.adjusted(hudBrightness) }, s.hasBuiltin
	case mediakeys.VolumeUp, mediakeys.VolumeDown:
		if len(s.audioKeyTargets()) == 0 {
			return nil, false // the sound plays elsewhere, at macOS's volume
		}
		delta := s.prefs.volumeStep()
		if k == mediakeys.VolumeDown {
			delta = -delta
		}
		return func() {
			for _, device := range s.audioKeyTargets() {
				device.setLocalVolume(min(max(device.volume+delta, 0), 100))
			}
			s.adjusted(hudVolume)
		}, false
	case mediakeys.Mute:
		if len(s.audioKeyTargets()) == 0 {
			return nil, false
		}
		return func() {
			for _, device := range s.audioKeyTargets() {
				if device.volume > 0 {
					device.unmuted = device.volume
					device.setLocalVolume(0)
				} else {
					v := device.unmuted
					if v == 0 {
						v = defaultUnmuted
					}
					device.setLocalVolume(v)
				}
			}
			s.adjusted(hudVolume)
		}, false
	}
	return nil, false
}

// onMediaKey is the event tap's handler.
func (s *store) onMediaKey(k mediakeys.Key, down bool) (take bool) {
	step, pass := s.mediaKey(k)
	if step == nil {
		return false
	}
	if down { // after the tap returns, as macOS holds the keys until then
		go s.post(step)
	}
	return !pass
}

// syncMediaKeys takes the keys or gives them back, as asked and allowed. It
// runs every second, as the user grants the permission in System Settings
// (or loses it to an update signed anew).
func (s *store) syncMediaKeys() {
	want := s.prefs.MediaKeys && s.bindShortcuts && mediakeys.Supported
	switch {
	case !want:
		mediakeys.Stop()
		s.mediaKeysDenied = false
	case !mediakeys.Trusted():
		mediakeys.Stop()
		s.mediaKeysDenied = true
	case !mediakeys.Running():
		s.mediaKeysDenied = mediakeys.Start(s.onMediaKey) != nil
	default:
		s.mediaKeysDenied = false
	}
}

// setMediaKeys turns taking the keys on or off, asking macOS for the
// permission the first time.
func (s *store) setMediaKeys(on bool) {
	s.prefs.MediaKeys, s.dirty = on, true
	if on && s.bindShortcuts && !mediakeys.Trusted() {
		mediakeys.RequestAccess()
	}
	s.syncMediaKeys()
}

// openAccessibility opens the Accessibility list in System Settings.
func openAccessibility() { _ = mygo.Shell.OpenExternal(mediakeys.SettingsURL) }

// Explicit bindings take precedence over group synchronization: only currently
// active audio outputs receive keyboard commands, never unrelated LAN devices.
func (s *store) audioKeyTargets() []*store {
	if !s.soundToDisplay() {
		return nil
	}
	devices := append([]*store{s}, s.peers...)
	var out []*store
	if len(s.prefs.AudioBindings) == 0 {
		if s.hasVolume && !s.offline {
			out = append(out, s)
		}
		if s.prefs.SyncVolume {
			for _, p := range s.peers {
				if p.hasVolume && !p.offline && !s.excludedAudio(p) {
					out = append(out, p)
				}
			}
		}
		return out
	}
	if s.activeAudioUIDs == nil {
		return nil
	}
	uids := s.activeAudioUIDs()
	for _, d := range devices {
		if d.link != nil && d.hasVolume && !d.offline && slices.Contains(uids, s.prefs.AudioBindings[d.link.Host]) {
			out = append(out, d)
		}
	}
	return out
}
