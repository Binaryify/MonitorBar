package main

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/locales"
)

// tab is a page of the panel: id is kept in the settings, label shown.
type tab struct {
	id    string
	label func() string
	icon  string
}

var tabs = []tab{
	{"overview", locales.TabOverview, "display"},
	{"picture", locales.TabPicture, "sliders"},
	{"modes", locales.TabModes, "aspect"},
	{"activity", locales.TabActivity, "chart"},
}

const arrowHeight = 10

// panel draws the store and reports the height it wants.
type panel struct {
	s *store
	// arrowX is where the arrow points, from the panel's left edge (macOS).
	arrowX float32
	// maxContent caps the scrolling area, so the panel fits the screen.
	maxContent float32
	// onHeight hears the panel's natural height once it changes.
	onHeight func(h int)

	height         int
	contentHeight  float32
	overviewHeight float32 // every tab takes it, once measured (#9)
	settle         int     // frames still to draw while the sizes catch up
	ownFrame       bool    // this frame was asked for by settle
}

func (p *panel) view(c *ui.Context) {
	c.SetTheme(darkTheme)
	if runtime.GOOS == "windows" {
		// An opaque root lets text use ClearType: subpixel glyphs need a
		// solid background. Windows 11 rounds and outlines the flyout itself.
		c.Root().Background(panelBottom)
	} else {
		c.Root().Background(ui.Transparent) // the arrow and the rounded corners show the desktop
	}
	shell := ui.Column(c).Width(panelWidth).TextColor(ink).FontSize(12)
	shell.Children(func() {
		if runtime.GOOS == "darwin" {
			p.arrow(c)
		}
		body := ui.Column(c).PaddingX(13).Gradient(panelTop, panelBottom, 135)
		if runtime.GOOS == "darwin" { // Windows 11 rounds and outlines the flyout itself
			body.Radius(16).Border(0.7, ui.RGBA(255, 255, 255, 0.12)).Clip()
		}
		body.Children(func() {
			p.header(c)
			p.tabs(c)
			p.content(c)
			p.updateBar(c)
			p.footer(c)
		})
	})
	// Sizes come from the previous frame's layout. After a frame something
	// else asked for (input, a result), draw one more to measure it, and
	// more while the sizes keep changing.
	if !p.ownFrame {
		p.settle = max(p.settle, 1)
	}
	p.ownFrame = p.settle > 0
	if p.settle > 0 {
		p.settle--
		c.Invalidate()
	}
	if h := int(shell.Bounds().H + 0.5); h > 0 && h != p.height {
		p.height = h
		if p.onHeight != nil {
			go mygo.RunOnMain(func() { p.onHeight(h) }) // resize after this frame
		}
	}
}

func (p *panel) arrow(c *ui.Context) {
	x := min(max(p.arrowX, 18), panelWidth-18)
	ui.Box(c).Height(arrowHeight).Draw(func(pt *ui.Painter, r ui.Rect) {
		var path ui.Path
		left, tip, right := r.X+x-11, r.X+x, r.X+x+11
		path.MoveTo(left, r.Y+r.H)
		path.QuadTo(tip-3, r.Y+r.H-2, tip, r.Y)
		path.QuadTo(tip+3, r.Y+r.H-2, right, r.Y+r.H)
		path.Close()
		pt.FillPath(&path, panelTop)
	})
}

func (p *panel) header(c *ui.Context) {
	s := p.s
	ui.Row(c).Gap(10).Padding(16, 0, 14, 0).Children(func() {
		ui.Box(c).Size(32, 32).Radius(9).Center().Gradient(accent, accentDeep, 180).Children(func() {
			icon(c, "display").FontSize(15).TextColor(ui.RGB(255, 255, 255))
		})
		ui.Column(c).Gap(3).Grow(1).Children(func() {
			ui.Text(c, or(s.monitor.Name, locales.MonitorUnnamed())).FontSize(16).FontWeight(600).LetterSpacing(-0.3).SingleLine()
			ui.Text(c, p.caption()).FontSize(10).FontWeight(500).TextColor(muted)
		})
		ui.Box(c).Size(24, 30).Center().TextColor(muted).Label(locales.MenuMore()).Cursor(ui.CursorPointer).Children(func() {
			icon(c, "ellipsis").FontSize(16)
		}).Menu(func(m *ui.Menu) {
			if m.Item(locales.MenuDetect()).Chosen() {
				s.detect()
			}
			if m.Item(locales.MenuDisplaySettings()).Chosen() {
				_ = mygo.Shell.OpenExternal(display.SettingsURL)
			}
			if m.Item(locales.MenuLaunchAtLogin()).Checked(s.launchAtLogin).Chosen() {
				s.toggleLaunchAtLogin()
			}
			if s.canUpdate && m.Item(locales.MenuCheckUpdates()).Chosen() {
				s.checkUpdate(true)
			}
			m.Submenu(locales.MenuLanguage(), func(m *ui.Menu) {
				if m.Item(locales.MenuLanguageSystem()).Checked(s.prefs.Language == "").Chosen() {
					s.setLanguage("")
				}
				m.Separator()
				for _, tag := range mygo.App.Languages() {
					if m.Item(i18n.Autonym(tag)).Checked(s.prefs.Language == tag).Chosen() {
						s.setLanguage(tag)
					}
				}
			})
			if m.Item(locales.MenuAbout()).Chosen() {
				go about() // the dialog waits for its answer: not on the main thread
			}
			m.Separator()
			if m.Item(locales.MenuQuit()).Chosen() {
				mygo.App.Quit()
			}
		})
		power := ui.Box(c).Size(32, 32).Radius(16).Center().Cursor(ui.CursorPointer).Tooltip(locales.SleepDisplay()).Label(locales.SleepDisplay())
		power.Background(ui.RGBA(255, 255, 255, 0.07))
		if power.Hovered() {
			power.Background(ui.RGBA(255, 255, 255, 0.12))
		}
		power.Children(func() { icon(c, "power").FontSize(15) })
		if power.Clicked() {
			s.sleepDisplay()
		}
	})
}

// caption sums the monitor up: "External display · HDMI · 4K · 160Hz".
func (p *panel) caption() string {
	parts := []string{locales.CaptionExternal()}
	if p.s.monitor.Connection != "" {
		parts = append(parts, p.s.monitor.Connection)
	}
	if label := display.SizeLabel(p.s.monitor.NativeSize(p.s.modes, p.s.current)); label != "" {
		parts = append(parts, label)
	}
	if p.s.hasCurrent { // the current refresh rate, not the fastest of any mode (issue #2)
		parts = append(parts, strconv.Itoa(p.s.current.RefreshRate)+"Hz")
	}
	return strings.Join(parts, " · ")
}

func (p *panel) tabs(c *ui.Context) {
	s := p.s
	ui.Row(c).Gap(4).Padding(4).Margin(0, 0, 12, 0).Radius(12).Background(ui.RGBA(255, 255, 255, 0.066)).Border(0.7, cardStroke).Children(func() {
		for _, t := range tabs {
			selected := s.currentTab().id == t.id
			e := ui.Box(c.Key(t.id)).Grow(1).Height(34).Radius(9).Center().Tooltip(t.label()).Label(t.label()).Cursor(ui.CursorPointer)
			if selected {
				e.Background(accent.Alpha(0.2)).TextColor(accent)
			} else {
				e.TextColor(muted)
			}
			e.Children(func() { icon(c, t.icon).FontSize(16) })
			if e.Clicked() && !selected {
				s.prefs.Tab, s.dirty, s.notice = t.id, true, ""
				s.record("")
			}
		}
	})
}

// content scrolls the tab when it is taller than the panel. Every tab is
// as tall as the overview, so switching tabs doesn't resize the panel and
// long ones like 画面 scroll; until the overview was shown, a tab takes its
// own height. The screen caps both.
func (p *panel) content(c *ui.Context) {
	s := p.s
	height := p.contentHeight
	if p.overviewHeight > 0 {
		height = p.overviewHeight
	}
	scroll := ui.Scroll(c.Key("content")).Height(min(max(height, 1), p.maxContent))
	scroll.Children(func() {
		inner := ui.Column(c).Gap(12).Padding(0, 0, 4, 0)
		inner.Children(func() {
			t := s.currentTab()
			if t.id != tabs[0].id {
				sectionTitle(c, t.label())
			}
			switch t.id {
			case "picture":
				p.picture(c)
			case "modes":
				p.modes(c)
			case "activity":
				p.activity(c)
			default:
				p.overview(c)
			}
			if s.notice != "" {
				ui.Text(c, s.notice).FontSize(11).TextColor(muted)
			}
		})
		if h := inner.Bounds().H; h > 0 && h != p.contentHeight {
			p.contentHeight, p.settle = h, 3
			c.Invalidate()
		}
		if s.currentTab().id == tabs[0].id {
			p.overviewHeight = p.contentHeight
		}
	})
}

// updateBar offers a newer version, when there is one.
func (p *panel) updateBar(c *ui.Context) {
	s := p.s
	if s.update == nil {
		return
	}
	ui.Row(c).Gap(8).Padding(10, 12).Margin(12, 0, 0, 0).Radius(10).Background(accent.Alpha(0.12)).
		Border(0.7, accent.Alpha(0.35)).AlignItems(ui.Center).Children(func() {
		icon(c, "sparkles").FontSize(13).TextColor(accent)
		ui.Text(c, locales.UpdateAvailable(s.update.Version)).FontSize(11).FontWeight(600).Grow(1)
		if s.installing {
			ui.Text(c, locales.UpdateProgress(s.progress)).FontSize(11).TextColor(muted)
			return
		}
		b := chip(c, true).Padding(4, 12)
		b.Children(func() { ui.Text(c, locales.UpdateInstall()).FontSize(11).FontWeight(500) })
		if b.Clicked() {
			s.installUpdate()
		}
	})
}

// footer tells which channels reach the monitor. With none, it says
// nothing: software dimming needs no explaining.
func (p *panel) footer(c *ui.Context) {
	s := p.s
	text := ""
	switch {
	case s.link != nil && s.ddc != nil:
		text = locales.FooterLanAndDdc()
	case s.link != nil:
		text = locales.FooterLan()
	case s.ddc != nil:
		text = locales.FooterDdc()
	case s.detectingAny():
		text = locales.FooterDetecting()
	}
	if text == "" {
		ui.Box(c).Height(12)
		return
	}
	dot := ui.RGB(52, 199, 89)
	if s.link == nil && s.ddc == nil {
		dot = ui.RGB(255, 204, 0)
	}
	ui.Row(c).Gap(6).Padding(12, 0).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, text).FontSize(10).TextColor(muted).Grow(1)
		if s.detectingAny() {
			ui.Spinner(c).Size(12, 12)
		}
	})
}

const projectURL = "https://github.com/Binaryify/MonitorBar"

// about tells the version, and offers the project's page.
func about() {
	r, err := mygo.Dialog.Message(mygo.MessageOptions{
		Title:   locales.MenuAbout(),
		Message: "MonitorBar " + mygo.App.Version(),
		Detail:  locales.AboutDetail() + "\n" + projectURL,
		Buttons: []string{locales.AboutOK(), locales.AboutHomepage()},
	})
	if err == nil && r.Button == 1 {
		_ = mygo.Shell.OpenExternal(projectURL)
	}
}

func (s *store) currentTab() tab {
	for _, t := range tabs {
		if t.id == s.prefs.Tab {
			return t
		}
	}
	return tabs[0]
}

// setLanguage shows the panel in tag, "" for the system's language. MyGo
// keeps it for the next launches; the settings keep it for the menu's mark.
func (s *store) setLanguage(tag string) {
	s.prefs.Language, s.dirty = tag, true
	mygo.App.SetLocale(tag)
}
