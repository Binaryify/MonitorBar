package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/usage"
)

// MARK: 概览

func (p *panel) overview(c *ui.Context) {
	s := p.s
	p.hero(c)
	if sources := s.sources(); len(sources) > 0 {
		card(c, func() {
			cardTitle(c, "输入源", "plug", s.inputChannel())
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
		if (sliderCard{key: "backlight", title: "亮度", caption: "硬件背光 · DDC/CI", icon: "sun",
			value: &s.brightness, lo: 0, hi: 100, step: 5}).show(c) {
			s.setBrightness(s.brightness)
		}
	} else if (sliderCard{key: "brightness", title: "亮度", caption: "软件调光 · Gamma", icon: "sun",
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
			caption = fmt.Sprintf("HiDPI · 渲染 %d×%d · %d Hz", m.PixelWidth, m.PixelHeight, m.RefreshRate)
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
			ui.Text(c, "当前画面").FontSize(9).Bold().LetterSpacing(1.1).TextColor(muted).Grow(1)
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
			icon(c, "volume").FontSize(14).TextColor(muted).Width(18)
			ui.Text(c, "显示器音量").FontSize(12).FontWeight(600).Grow(1)
			ui.Text(c, fmt.Sprint(int(math.Round(s.volume)))).FontSize(20).FontWeight(600).FontFeatures("tnum")
		})
		if ui.Slider(c.Key("volume"), &s.volume, 0, 100).Label("显示器音量").Changed() {
			s.setVolume(s.volume)
		}
	})
}

func (p *panel) scenes(c *ui.Context) {
	s := p.s
	card(c, func() {
		cardTitle(c, "场景", "sparkles", "亮度 + 色温")
		ui.Row(c).Gap(6).Children(func() {
			for _, sc := range scenes {
				e := chip(c.Key(sc.name), s.isActive(sc)).Grow(1).Height(50).Gap(5)
				e.Children(func() {
					icon(c, sc.icon).FontSize(14)
					ui.Text(c, sc.name).FontSize(10).FontWeight(500)
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
	p.autoSwitch(c)
	if (sliderCard{key: "soft", title: "软件调光", caption: "在硬件亮度基础上再压暗，适合夜间", icon: "contrast",
		value: &s.prefs.SoftBrightness, lo: 20, hi: 100, step: 5}).show(c) {
		s.applyGamma()
	}
	if (sliderCard{key: "warmth", title: "色温", caption: "向右更暖，减少蓝光", icon: "thermometer",
		value: &s.prefs.Warmth, lo: 0, hi: 100, step: 5, format: func(v float64) string {
			if v < 1 {
				return "标准"
			}
			return fmt.Sprint(int(math.Round(v)))
		}}).show(c) {
		s.applyGamma()
	}
	for i := range s.controls { // only what the monitor answers over DDC/CI
		ctl := &s.controls[i]
		if (sliderCard{key: "ddc-" + ctl.name, title: ctl.name, caption: "硬件 · DDC/CI", icon: "sliders",
			value: &ctl.value, lo: 0, hi: float64(ctl.max), step: 1}).show(c) {
			s.setControl(i)
		}
	}
	reset := chip(c, false).Height(30)
	reset.Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			icon(c, "rotate").FontSize(11)
			ui.Text(c, "恢复软件调整").FontSize(11).FontWeight(500)
		})
	})
	if reset.Clicked() {
		s.resetSoftware()
	}
}

// autoSwitch is the card that switches to the day and night scenes at set
// times.
func (p *panel) autoSwitch(c *ui.Context) {
	s := p.s
	names := make([]string, len(scenes))
	for i, sc := range scenes {
		names[i] = sc.name
	}
	card(c, func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			icon(c, "clock").FontSize(13)
			ui.Text(c, "自动切换").FontSize(12).FontWeight(600).Grow(1)
			if ui.Switch(c.Key("auto"), &s.prefs.AutoSwitch).Label("启用自动切换").Changed() {
				s.autoChanged()
			}
		})
		if !s.prefs.AutoSwitch {
			ui.Text(c, "按时间在白天和夜晚的场景之间切换").FontSize(10).TextColor(muted)
			return
		}
		row := func(key, label string, at *time.Time, scene *string) {
			ui.Row(c.Key(key)).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(11).Width(56)
				if ui.TimeInput(c.Key(key+"-time"), at).Label(label + "开始").Changed() {
					s.autoChanged()
				}
				ui.Spacer(c)
				if ui.Select(c.Key(key+"-scene"), scene, names).Label(label + "场景").Width(96).Changed() {
					s.autoChanged()
				}
			})
		}
		row("day", "白天", &s.dayStart, &s.prefs.DayScene)
		row("night", "夜晚", &s.nightStart, &s.prefs.NightScene)
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
	if rates := display.RefreshRates(s.modes, s.current); len(rates) > 1 {
		card(c, func() {
			cardTitle(c, "刷新率", "gauge", s.current.Title())
			ui.Grid(c).Columns(4).Gap(6).Children(func() {
				for _, m := range rates {
					if chipText(c.Key(m.ID()), fmt.Sprintf("%d Hz", m.RefreshRate), m.RefreshRate == rate) {
						s.applyMode(m)
					}
				}
			})
		})
	}
	p.resolutions(c, "HiDPI（清晰）", "zoom", display.Resolutions(s.modes, true, rate))
	p.resolutions(c, "标准", "layers", display.Resolutions(s.modes, false, rate))
}

func (p *panel) resolutions(c *ui.Context, title, iconName string, modes []display.Mode) {
	s := p.s
	if len(modes) == 0 {
		return
	}
	card(c.Key(title), func() {
		cardTitle(c, title, iconName, fmt.Sprintf("%d 种", len(modes)))
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
						detail = fmt.Sprintf("渲染 %d×%d", m.PixelWidth, m.PixelHeight)
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
		statTile(c, "今日亮屏", duration(today.Awake), "sun")
		statTile(c, "今日使用", duration(today.Active), "keyboard")
	})
	card(c, func() {
		cardTitle(c, "最近 7 天", "calendar", "日均 "+duration(total/max(recorded, 1)))
		ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
			for _, d := range s.usage {
				p.dayBar(c.Key(d.Day), d, d.Day == today.Day, peak)
			}
		})
		ui.Row(c).Gap(12).Children(func() {
			legend(c, accent.Alpha(0.3), "亮屏")
			legend(c, accent, "有键鼠操作")
		})
	})
	ui.Text(c, "亮屏：显示器点亮的时长；使用：两分钟内有键盘或鼠标操作的时长。MonitorBar 运行时才会记录。").FontSize(10).TextColor(muted)
}

func (p *panel) dayBar(c *ui.Context, d usage.Day, today bool, peak int) {
	const track = 110
	ui.Column(c).Grow(1).Gap(6).AlignItems(ui.Center).
		Tooltip(fmt.Sprintf("%s  亮屏 %s · 使用 %s", d.Day, duration(d.Awake), duration(d.Active))).Children(func() {
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
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("%d 小时 %d 分", minutes/60, minutes%60)
}

func weekday(day string) string {
	t, err := time.ParseInLocation(time.DateOnly, day, time.Local)
	if err != nil {
		return ""
	}
	if day == time.Now().Format(time.DateOnly) {
		return "今天"
	}
	return [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[t.Weekday()]
}
