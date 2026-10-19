package main

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mediakeys"
	"github.com/binaryify/MonitorBar/internal/usage"
	"github.com/binaryify/MonitorBar/locales"
)

// MARK: 概览

func (p *panel) overview(c *ui.Context) {
	s := p.s
	p.hero(c)
	if sources := s.sources(); len(sources) > 0 {
		card(c, func() {
			cardTitle(c, locales.Inputs(), "plug", s.inputChannel())
			ui.Grid(c).Columns(4).Gap(6).Children(func() {
				for _, src := range sources {
					if chipText(c.Key(src.ID), src.Name, s.input == src.ID) {
						s.selectInput(src)
					}
				}
			})
		})
	}
	if s.hasBrightness {
		combined := s.combinedBrightness()
		caption := locales.BrightnessCombined()
		switch {
		case combined < switchover-0.5:
			caption = locales.BrightnessMinimumSoftware()
		case combined <= switchover+0.5:
			caption = locales.BrightnessMinimum()
		case s.prefs.FollowBuiltin && s.canFollow():
			caption = locales.BrightnessFollowing()
		}
		if (sliderCard{key: "backlight", title: locales.Brightness(), caption: caption, icon: "sun",
			value: &combined, lo: 0, hi: 100, step: 5}).show(c) {
			s.setCombined(combined)
		}
	} else if (sliderCard{key: "brightness", title: locales.Brightness(), caption: locales.BrightnessSoftware(), icon: "sun",
		value: &s.prefs.SoftBrightness, lo: 20, hi: 100, step: 5}).show(c) {
		s.applyGamma()
	}
	if s.hasVolume {
		p.volume(c)
	}
	p.scenes(c)
}

// hero is a drawing of the monitor showing its current mode.
func (p *panel) hero(c *ui.Context) {
	s := p.s
	glow := float32(s.effectiveBrightness() / 100)
	title, caption := "—", ""
	if s.hasCurrent {
		m := s.current
		title, caption = m.Title(), fmt.Sprintf("%d Hz", m.RefreshRate)
		if m.HiDPI() {
			caption = locales.ModeHiDPI(m.PixelWidth, m.PixelHeight, m.RefreshRate)
		}
	}
	ui.Column(c).Children(func() {
		// A soft elliptical glow under the screen, brighter as the picture is.
		ui.Column(c).Height(186).AlignItems(ui.Center).Justify(ui.Center).Draw(func(pt *ui.Painter, r ui.Rect) {
			oval := ui.Rect{X: r.X + r.W/2 - 125, Y: r.Y + r.H/2 - 44, W: 250, H: 100}
			pt.Shadow(oval, 50, 0, 0, 50, 6, accent.Alpha(0.1+0.26*glow))
		}).Children(func() {
			ui.Box(c).Size(268, 151).Radius(8).Padding(4).Background(ui.RGB(4, 4, 4)).Border(0.8, ui.RGBA(255, 255, 255, 0.16)).Children(func() {
				ui.Column(c).Fill().Radius(5).Center().Gap(5).
					Gradient(accent.Alpha(0.05+0.3*glow), ui.RGB(0, 0, 0), 135).Children(func() {
					ui.Text(c, title).FontSize(22).FontWeight(600).LetterSpacing(-0.4).FontFeatures("tnum")
					ui.Text(c, caption).FontSize(10).FontWeight(500).TextColor(ui.RGBA(255, 255, 255, 0.6))
				})
			})
			ui.Box(c).Size(30, 18).Gradient(ui.RGB(51, 51, 51), ui.RGB(26, 26, 26), 180)
			ui.Box(c).Size(108, 5).Radius(3).Background(ui.RGB(51, 51, 51))
		})
		ui.Row(c).Gap(5).Padding(8, 6, 0, 6).Children(func() {
			ui.Box(c).Size(4, 4).Radius(2).Background(accent)
			ui.Text(c, locales.CurrentMode()).FontSize(9).Bold().LetterSpacing(1.1).TextColor(muted).Grow(1)
			if s.monitor.Connection != "" {
				ui.Text(c, s.monitor.Connection).FontSize(9).FontWeight(600).TextColor(accent)
			}
		})
	})
}

func (p *panel) volume(c *ui.Context) {
	s := p.s
	card(c, func() {
		ui.Row(c).Gap(8).Children(func() {
			name, label := "volume", locales.Mute()
			if s.volume == 0 {
				name, label = "volumeOff", locales.Unmute()
			}
			mute := chip(c.Key("mute"), s.volume == 0).Size(26, 22).Label(label)
			mute.Children(func() { icon(c, name).FontSize(12) })
			if mute.Clicked() {
				s.toggleMute()
			}
			ui.Text(c, locales.Volume()).FontSize(12).FontWeight(600).Grow(1)
			ui.Text(c, fmt.Sprint(int(math.Round(s.volume)))).FontSize(20).FontWeight(600).FontFeatures("tnum")
		})
		if ui.Slider(c.Key("volume"), &s.volume, 0, 100).Label(locales.Volume()).Changed() {
			s.setVolume(s.volume)
		}
	})
}

func (p *panel) scenes(c *ui.Context) {
	s := p.s
	card(c, func() {
		cardTitle(c, locales.Scenes(), "sparkles", locales.ScenesNote())
		ui.Row(c).Gap(6).Children(func() {
			for _, sc := range scenes {
				e := chip(c.Key(sc.id), s.isActive(sc)).Grow(1).Height(50).Gap(5)
				e.Children(func() {
					icon(c, sc.icon).FontSize(14)
					ui.Text(c, sc.label()).FontSize(10).FontWeight(500)
				})
				if e.Clicked() {
					s.apply(sc)
				}
			}
		})
	})
}

// MARK: 画面

func (p *panel) picture(c *ui.Context) {
	s := p.s
	p.scenes(c)
	if s.canFollow() {
		p.followBuiltin(c)
	}
	p.autoSwitch(c)
	if (sliderCard{key: "warmth", title: locales.Warmth(), caption: locales.WarmthNote(), icon: "thermometer",
		value: &s.prefs.Warmth, lo: 0, hi: 100, step: 5, format: func(v float64) string {
			if v < 1 {
				return locales.WarmthNeutral()
			}
			return fmt.Sprint(int(math.Round(v)))
		}}).show(c) {
		s.applyGamma()
	}
	for i := range s.controls { // only what the monitor answers over DDC/CI
		ctl := &s.controls[i]
		if (sliderCard{key: fmt.Sprintf("ddc-%02x", ctl.code), title: ctl.label(), caption: locales.ControlNote(), icon: "sliders",
			value: &ctl.value, lo: 0, hi: float64(ctl.max), step: 1}).show(c) {
			s.setControl(i)
		}
	}
	reset := chip(c, false).Height(30)
	reset.Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			icon(c, "rotate").FontSize(11)
			ui.Text(c, locales.ResetSoftware()).FontSize(11).FontWeight(500)
		})
	})
	if reset.Clicked() {
		s.resetSoftware()
	}
	p.shortcuts(c)
}

// shortcuts lists the brightness, volume and input shortcuts. Clicking one records
// keys for it: a combination with Ctrl, Option or Command, or a function
// key, sets it, Esc cancels and Delete clears it.
func (p *panel) shortcuts(c *ui.Context) {
	s := p.s
	card(c, func() {
		cardTitle(c, locales.Shortcuts(), "keyboard", locales.ShortcutsNote())
		for _, sc := range s.panelShortcuts() {
			ui.Row(c.Key(sc.id)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, sc.title()).FontSize(11).Grow(1)
				recording := s.recording == sc.id
				label := keysLabel(s.prefs.keys(sc))
				if recording {
					label = locales.ShortcutRecording()
				}
				e := chip(c.Key(sc.id+"-keys"), recording).Width(132).Height(26).Label(locales.ShortcutKeys(sc.title())).Focusable()
				e.Children(func() { ui.Text(c, label).FontSize(11).FontWeight(500).SingleLine() })
				e.HandleInput(func(ev ui.InputEvent) bool {
					if s.recording != sc.id || ev.Kind != ui.InputKeyDown {
						return false
					}
					key := ev.Key
					if key == ui.KeyUnknown {
						key = ev.PhysicalKey
					}
					switch {
					case ev.Mods == 0 && key == ui.KeyEscape:
						s.record("")
					case ev.Mods == 0 && (key == ui.KeyBackspace || key == ui.KeyDelete):
						s.setKeys(sc.id, "")
					default:
						keys, ok := recordKey(ev.Mods, key)
						if !ok {
							return true // keep waiting for a combination
						}
						s.setKeys(sc.id, keys)
					}
					s.redraw()
					return true
				})
				if e.Clicked() {
					s.record(sc.id)
				}
				if recording {
					e.Focus()
				}
			})
		}
		if s.recording != "" {
			hint := locales.ShortcutHint()
			if runtime.GOOS == "darwin" {
				hint = locales.ShortcutHintMac()
			}
			ui.Text(c, hint).FontSize(10).TextColor(muted)
		}
		if mediakeys.Supported {
			p.mediaKeys(c)
		}
	})
}

// mediaKeys is the advanced choice to take the keyboard's brightness and
// volume keys, which needs the Accessibility permission.
func (p *panel) mediaKeys(c *ui.Context) {
	s := p.s
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.MediaKeys()).FontSize(11).Grow(1)
		on := s.prefs.MediaKeys
		if ui.Switch(c.Key("media-keys"), &on).Label(locales.MediaKeys()).Changed() {
			s.setMediaKeys(on)
		}
	})
	if !s.mediaKeysDenied {
		ui.Text(c, locales.MediaKeysNote()).
			FontSize(10).TextColor(muted)
		return
	}
	ui.Text(c, locales.MediaKeysDenied()).
		FontSize(10).TextColor(accent)
	if chipText(c.Key("media-keys-settings"), locales.MediaKeysSettings(), false) {
		openAccessibility()
	}
}

// followBuiltin is the card that has the backlight follow a MacBook's own
// display, whose brightness macOS sets by the ambient light.
func (p *panel) followBuiltin(c *ui.Context) {
	s := p.s
	card(c, func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			icon(c, "sun").FontSize(13)
			ui.Text(c, locales.Follow()).FontSize(12).FontWeight(600).Grow(1)
			if ui.Switch(c.Key("follow"), &s.prefs.FollowBuiltin).Label(locales.FollowSwitch()).Changed() {
				s.followChanged()
			}
		})
		if !s.prefs.FollowBuiltin {
			ui.Text(c, locales.FollowNote()).FontSize(10).TextColor(muted)
			return
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.FollowRange()).FontSize(11)
			ui.Spacer(c)
			ui.Textf(c, "%d – %d", int(s.prefs.FollowMin), int(s.prefs.FollowMax)).FontSize(11).FontFeatures("tnum")
		})
		if ui.RangeSlider(c.Key("follow-range"), &s.prefs.FollowMin, &s.prefs.FollowMax, 0, 100, 1).Label(locales.FollowRange()).Changed() {
			s.followChanged()
		}
		if s.lastBuiltin >= 0 {
			ui.Text(c, locales.FollowStatus(int(math.Round(s.lastBuiltin*100)), int(math.Round(s.followTarget)))).
				FontSize(10).TextColor(muted)
		}
	})
}

// autoSwitch is the card that switches to the day and night scenes at set
// times.
func (p *panel) autoSwitch(c *ui.Context) {
	s := p.s
	labels := make([]string, len(scenes))
	for i, sc := range scenes {
		labels[i] = sc.label()
	}
	card(c, func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			icon(c, "clock").FontSize(13)
			ui.Text(c, locales.Auto()).FontSize(12).FontWeight(600).Grow(1)
			if ui.Switch(c.Key("auto"), &s.prefs.AutoSwitch).Label(locales.AutoSwitch()).Changed() {
				s.autoChanged()
			}
		})
		if !s.prefs.AutoSwitch {
			ui.Text(c, locales.AutoNote()).FontSize(10).TextColor(muted)
			return
		}
		row := func(key, label string, at *time.Time, scene *string) {
			ui.Row(c.Key(key)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(11).Width(56)
				if ui.TimeInput(c.Key(key+"-time"), at).Label(locales.AutoStart(label)).Changed() {
					s.autoChanged()
				}
				ui.Spacer(c)
				// The select shows the labels; the settings keep the scene's id.
				chosen := sceneByID(*scene).label()
				if ui.Select(c.Key(key+"-scene"), &chosen, labels).Label(locales.AutoScene(label)).Width(96).Changed() {
					*scene = scenes[slices.Index(labels, chosen)].id
					s.autoChanged()
				}
			})
		}
		row("day", locales.AutoDay(), &s.dayStart, &s.prefs.DayScene)
		row("night", locales.AutoNight(), &s.nightStart, &s.prefs.NightScene)
		ui.Text(c, s.autoStatus()).FontSize(10).TextColor(muted)
	})
}

// MARK: 分辨率

func (p *panel) modes(c *ui.Context) {
	s := p.s
	rate := 60
	if s.hasCurrent {
		rate = s.current.RefreshRate
	}
	if s.link != nil { // only the HyperOS remote can switch the monitor's own mode
		card(c, func() {
			trailing := ""
			if s.switchingTo != "" {
				trailing = locales.CompetitiveSwitching()
			}
			cardTitle(c, locales.Competitive(), "sparkles", trailing)
			on := s.competitive()
			ui.Grid(c).Columns(2).Gap(6).Children(func() {
				if chipText(c.Key("4k"), "4K 160Hz", !on) {
					s.setCompetitive(false)
				}
				if chipText(c.Key("320"), "1080p 320Hz", on) {
					s.setCompetitive(true)
				}
			})
		})
	}
	if rates := display.RefreshRates(s.modes, s.current); len(rates) > 1 {
		card(c, func() {
			cardTitle(c, locales.RefreshRate(), "gauge", s.current.Title())
			ui.Grid(c).Columns(4).Gap(6).Children(func() {
				for _, m := range rates {
					if chipText(c.Key(m.ID()), fmt.Sprintf("%d Hz", m.RefreshRate), m.RefreshRate == rate) {
						s.applyMode(m)
					}
				}
			})
		})
	}
	p.resolutions(c, locales.ModesHiDPI(), "zoom", display.Resolutions(s.modes, true, rate))
	p.resolutions(c, locales.ModesStandard(), "layers", display.Resolutions(s.modes, false, rate))
}

func (p *panel) resolutions(c *ui.Context, title, iconName string, modes []display.Mode) {
	s := p.s
	if len(modes) == 0 {
		return
	}
	card(c.Key(iconName), func() {
		cardTitle(c, title, iconName, locales.ModesCount(len(modes)))
		ui.Column(c).Gap(4).Children(func() {
			for _, m := range modes {
				selected := s.hasCurrent && m.SizeKey() == s.current.SizeKey()
				row := ui.Row(c.Key(m.ID())).Height(28).PaddingX(9).Gap(8).Radius(8).Cursor(ui.CursorPointer)
				if selected {
					row.Background(accent.Alpha(0.14))
				} else if row.Hovered() {
					row.Background(ui.RGBA(255, 255, 255, 0.05))
				}
				row.Children(func() {
					t := ui.Text(c, m.Title()).FontSize(12).FontFeatures("tnum").Grow(1)
					if selected {
						t.FontWeight(600)
					}
					detail := fmt.Sprintf("%d Hz", m.RefreshRate)
					if m.HiDPI() {
						detail = locales.ModeRendered(m.PixelWidth, m.PixelHeight)
					}
					ui.Text(c, detail).FontSize(10).TextColor(muted)
					check := icon(c, "check").FontSize(10).TextColor(accent)
					if !selected {
						check.Opacity(0)
					}
				})
				if row.Clicked() {
					s.applyMode(m)
				}
			}
		})
	})
}

// MARK: 屏幕时间

func (p *panel) activity(c *ui.Context) {
	s := p.s
	today := s.usage[len(s.usage)-1]
	peak, total, recorded := 3600, 0, 0
	for _, d := range s.usage {
		peak = max(peak, d.Awake)
		if d.Awake > 0 {
			total, recorded = total+d.Awake, recorded+1
		}
	}
	ui.Row(c).Gap(10).Children(func() {
		statTile(c, locales.ActivityAwakeToday(), duration(today.Awake), "sun")
		statTile(c, locales.ActivityActiveToday(), duration(today.Active), "keyboard")
	})
	card(c, func() {
		cardTitle(c, locales.ActivityWeek(), "calendar", locales.ActivityAverage(duration(total/max(recorded, 1))))
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			for _, d := range s.usage {
				p.dayBar(c.Key(d.Day), d, d.Day == today.Day, peak)
			}
		})
		ui.Row(c).Gap(12).Children(func() {
			legend(c, accent.Alpha(0.3), locales.ActivityAwake())
			legend(c, accent, locales.ActivityActive())
		})
	})
	ui.Text(c, locales.ActivityNote()).FontSize(10).TextColor(muted)
}

func (p *panel) dayBar(c *ui.Context, d usage.Day, today bool, peak int) {
	const track = 110
	ui.Column(c).Grow(1).Gap(6).AlignItems(ui.Center).
		Tooltip(locales.ActivityDay(d.Day, duration(d.Awake), duration(d.Active))).Children(func() {
		ui.Box(c).FillWidth().Height(track).Radius(4).Background(ui.RGBA(255, 255, 255, 0.06)).Children(func() {
			for _, bar := range []struct {
				seconds int
				color   ui.Color
			}{{d.Awake, accent.Alpha(0.3)}, {d.Active, accent}} {
				ui.Box(c).Absolute().Left(0).Bottom(0).FillWidth().Height(track * float32(bar.seconds) / float32(peak)).Radius(4).Background(bar.color)
			}
		})
		label := ui.Text(c, weekday(d.Day)).FontSize(9).TextColor(muted)
		if today {
			label.Bold().TextColor(accent)
		}
	})
}

func statTile(c *ui.Context, title, value, iconName string) {
	card(c, func() {
		ui.Row(c).Gap(4).TextColor(muted).Children(func() {
			icon(c, iconName).FontSize(10)
			ui.Text(c, title).FontSize(10).FontWeight(500)
		})
		ui.Text(c, value).FontSize(22).FontWeight(600).FontFeatures("tnum")
	}).Grow(1)
}

func legend(c *ui.Context, color ui.Color, title string) {
	ui.Row(c).Gap(4).Children(func() {
		ui.Box(c).Size(8, 8).Radius(2).Background(color)
		ui.Text(c, title).FontSize(9).TextColor(muted)
	})
}

func duration(seconds int) string {
	minutes := seconds / 60
	if minutes < 60 {
		return locales.DurationMinutes(minutes)
	}
	return locales.DurationHours(minutes/60, minutes%60)
}

func weekday(day string) string {
	t, err := time.ParseInLocation(time.DateOnly, day, time.Local)
	if err != nil {
		return ""
	}
	if day == time.Now().Format(time.DateOnly) {
		return locales.Today()
	}
	return i18n.FormatDate(i18n.Locale(), t, i18n.DateOptions{Skeleton: "EEE"})
}
