package main

import (
	"fmt"
	"net"

	"github.com/binaryify/MonitorBar/internal/mitv"

	"github.com/binaryify/MonitorBar/locales"
	"github.com/egoist/mygo/ui"
)

func (p *panel) dualControls(c *ui.Context) {
	s := p.s
	card(c, func() {
		ui.Text(c, locales.DualTitle()).FontSize(12).FontWeight(600)
		for i, m := range s.screens {
			if chipText(c.Key(fmt.Sprint("screen", i)), s.screenLabel(i, m.Key(), m.Name), s.monitor.Same(m)) {
				s.selectScreen(m)
			}
		}
		ui.Text(c, locales.DualHint()).FontSize(10)
		for i, m := range s.screens {
			include := !s.prefs.ExcludedScreens[m.Key()]
			if labeledSwitch(c.Key(fmt.Sprint("include-screen", i)), fmt.Sprintf("%s %d", locales.SyncMember(), i+1), &include) {
				if s.prefs.ExcludedScreens == nil {
					s.prefs.ExcludedScreens = map[string]bool{}
				}
				s.prefs.ExcludedScreens[m.Key()] = !include
				s.dirty = true
				s.save()
			}
		}

		for _, o := range []struct {
			key, label string
			value      *bool
			apply      func()
		}{
			{"volume", locales.SyncVolume(), &s.prefs.SyncVolume, func() {
				if s.hasVolume {
					s.setVolume(s.volume)
				}
			}},
			{"picture", locales.SyncPicture(), &s.prefs.SyncPicture, s.applyGamma},
			{"resolution", locales.SyncResolution(), &s.prefs.SyncResolution, func() {
				if s.hasCurrent {
					s.applyMode(s.current)
				}
			}},
		} {
			if labeledSwitch(c.Key(o.key), o.label, o.value) {
				s.dirty = true
				s.save()
				if *o.value {
					o.apply()
				}
			}
		}
		if s.prefs.SyncVolume {
			if labeledSwitch(c.Key("delta"), locales.SyncDelta(), &s.prefs.SyncDelta) {
				s.dirty = true
				s.save()
			}
		}

		if chipText(c.Key("rescan-dual"), locales.DualDiscover(), false) {
			s.connect()
		}
		for _, device := range append([]*store{s}, s.peers...) {
			if device.link == nil {
				continue
			}
			host := device.link.Host
			name := s.prefs.DeviceNames[host]
			title := name
			if title == "" {
				title = host
			}
			if chipText(c.Key("device-toggle-"+host), title, p.expandedDevice == host) {
				if p.expandedDevice == host {
					p.expandedDevice = ""
				} else {
					p.expandedDevice = host
				}
			}
			status := locales.LanOnline()
			if device.offline {
				status = locales.LanOffline()
			}
			ui.Text(c, host+" · "+status).FontSize(10).TextColor(muted)
			if p.expandedDevice != host {
				continue
			}
			ui.Text(c, locales.DeviceName()).FontSize(11)
			if ui.TextInput(c.Key("name-"+host), &name).Label(locales.DeviceName()).Changed() {
				if s.prefs.DeviceNames == nil {
					s.prefs.DeviceNames = map[string]string{}
				}
				s.prefs.DeviceNames[host] = name
				s.dirty = true
				s.save()
			}

			include := !s.prefs.ExcludedAudio[host]
			if labeledSwitch(c.Key("include-audio"+host), locales.SyncMember(), &include) {
				if s.prefs.ExcludedAudio == nil {
					s.prefs.ExcludedAudio = map[string]bool{}
				}
				s.prefs.ExcludedAudio[host] = !include
				s.dirty = true
				s.save()
			}

			for i, m := range s.screens {
				key := m.Key()
				if chipText(c.Key(fmt.Sprintf("bind-%s-%d", host, i)), fmt.Sprintf("%s %d", locales.BindScreen(), i+1), s.prefs.ScreenBindings[host] == key) {
					if s.prefs.ScreenBindings == nil {
						s.prefs.ScreenBindings = map[string]string{}
					}
					s.prefs.ScreenBindings[host] = key
					s.dirty = true
					s.save()
				}
			}

			ui.Text(c, locales.AudioBinding()).FontSize(11)
			uid := s.prefs.AudioBindings[host]
			if ui.TextInput(c.Key("audio-"+host), &uid).Label(locales.AudioBinding()).Changed() {
				if s.prefs.AudioBindings == nil {
					s.prefs.AudioBindings = map[string]string{}
				}
				if uid == "" {
					delete(s.prefs.AudioBindings, host)
				} else {
					s.prefs.AudioBindings[host] = uid
				}
				s.dirty = true
				s.save()
			}
			var outputs []string
			if s.activeAudioUIDs != nil {
				outputs = s.activeAudioUIDs()
			}
			for i, output := range outputs {
				if chipText(c.Key(host+output), fmt.Sprintf("%s %d", locales.AudioOutput(), i+1), uid == output) {
					if s.prefs.AudioBindings == nil {
						s.prefs.AudioBindings = map[string]string{}
					}
					s.prefs.AudioBindings[host] = output
					s.dirty = true
					s.save()
				}
			}

			if device.hostEdit == "" {
				device.hostEdit = host
			}
			ui.Text(c, locales.LanAddress()).FontSize(11)
			ui.TextInput(c.Key("host-"+host), &device.hostEdit).Label(locales.LanAddress())
			if chipText(c.Key("replace-"+host), locales.LanReplace(), false) {
				address := device.hostEdit
				valid := net.ParseIP(address) != nil
				for _, other := range append([]*store{s}, s.peers...) {
					if other != device && other.link != nil && other.link.Host == address {
						valid = false
					}
				}
				if address == host {
					continue
				}
				if valid && !device.chasing && !device.readingVolume && !s.detecting {
					s.prefs.moveDevice(host, address)
					p.expandedDevice = address

					device.link = mitv.NewBound(address, s.prefs.DeviceMACs[address])
					device.offline = true
					device.readVolume()
					s.dirty = true
					s.save()
				} else {
					s.notice = locales.LanAddressInvalid()
				}
			}

		}
		if chipText(c.Key("presets-toggle"), locales.PresetList(), p.showPresets) {
			p.showPresets = !p.showPresets
		}
		if p.showPresets {
			p.presetControls(c)
		}
		for _, peer := range s.peers {
			if peer.notice != "" && !peer.offline {
				ui.Text(c, peer.link.Host+": "+peer.notice).FontSize(10)
			}
		}
		ui.Text(c, locales.DualNote()).FontSize(10)
	})
}

// Label() is accessibility metadata, so switches also need visible text.
func labeledSwitch(c *ui.Context, label string, value *bool) bool {
	changed := false
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).Grow(1).MinWidth(0)
		changed = ui.Switch(c.Key("switch"), value).Label(label).Changed()
	})
	return changed
}
