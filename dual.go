package main

import (
	"fmt"
	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"net"

	"github.com/binaryify/MonitorBar/locales"
	"github.com/egoist/mygo/ui"
)

func (p *panel) dualControls(c *ui.Context) {
	s := p.s
	card(c, func() {
		ui.Text(c, locales.DualTitle()).FontSize(12).FontWeight(600)
		for i, m := range s.screens {
			if chipText(c.Key(fmt.Sprint("screen", i)), fmt.Sprintf("%d · %s", i+1, m.Name), s.monitor.Same(m)) {
				s.monitor = m
				s.modes = m.Modes()
				s.current, s.hasCurrent = m.CurrentMode()
				s.probeDDC()
			}
		}
		ui.Text(c, locales.DualHint()).FontSize(10)
		for _, o := range []struct {
			label string
			value *bool
		}{{locales.SyncVolume(), &s.prefs.SyncVolume}, {locales.SyncPicture(), &s.prefs.SyncPicture}, {locales.SyncResolution(), &s.prefs.SyncResolution}} {
			ui.Row(c).Children(func() {
				ui.Text(c, o.label).Grow(1)
				if ui.Switch(c.Key(o.label), o.value).Label(o.label).Changed() {
					s.dirty = true
					s.save()
					if *o.value {
						switch o.label {
						case locales.SyncVolume():
							if s.hasVolume {
								s.setVolume(s.volume)
							}
						case locales.SyncPicture():
							s.applyGamma()
						case locales.SyncResolution():
							if s.hasCurrent {
								s.applyMode(s.current)
							}
						}
					}
				}
			})
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
			for _, output := range display.ActiveDisplayAudioUIDs() {
				if chipText(c.Key(host+output), output, uid == output) {
					if s.prefs.AudioBindings == nil {
						s.prefs.AudioBindings = map[string]string{}
					}
					s.prefs.AudioBindings[host] = output
					s.dirty = true
					s.save()
				}
			}

			status := locales.LanOnline()
			if device.offline {
				status = locales.LanOffline()
			}
			ui.Text(c, status).FontSize(10)
			if device.hostEdit == "" {
				device.hostEdit = host
			}
			ui.TextInput(c.Key("host-"+host), &device.hostEdit).Label(locales.LanAddress())
			if chipText(c.Key("replace-"+host), locales.LanReplace(), false) {
				address := device.hostEdit
				valid := net.ParseIP(address) != nil
				for _, other := range append([]*store{s}, s.peers...) {
					if other != device && other.link != nil && other.link.Host == address {
						valid = false
					}
				}
				if valid && !device.chasing && !device.readingVolume {
					if s.prefs.DeviceNames == nil {
						s.prefs.DeviceNames = map[string]string{}
					}
					if s.prefs.AudioBindings != nil {
						s.prefs.AudioBindings[address] = s.prefs.AudioBindings[host]
						delete(s.prefs.AudioBindings, host)
					}
					s.prefs.DeviceNames[address] = name
					delete(s.prefs.DeviceNames, host)
					for i, h := range s.prefs.KnownHosts {
						if h == host {
							s.prefs.KnownHosts[i] = address
						}
					}
					if device == s {
						s.prefs.NetworkHost = address
					}
					device.link = mitv.New(address)
					device.offline = true
					device.readVolume()
					s.dirty = true
					s.save()
				} else {
					s.notice = locales.LanAddressInvalid()
				}
			}

			ui.Text(c, host).FontSize(10)
			if ui.TextInput(c.Key("name-"+host), &name).Label(locales.DeviceName()).Changed() {
				if s.prefs.DeviceNames == nil {
					s.prefs.DeviceNames = map[string]string{}
				}
				s.prefs.DeviceNames[host] = name
				s.dirty = true
				s.save()
			}
		}
		p.mediaKeys(c)
		for _, peer := range s.peers {
			if peer.notice != "" {
				ui.Text(c, peer.link.Host+": "+peer.notice).FontSize(10)
			}
		}
		ui.Text(c, locales.DualNote()).FontSize(10)
	})
}
