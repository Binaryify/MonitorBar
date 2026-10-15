package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

const panelWidth = 360

var (
	accent      = ui.RGB(255, 105, 0) // Xiaomi orange
	accentDeep  = ui.RGB(255, 69, 0)
	ink         = ui.RGBA(255, 255, 255, 0.87)
	muted       = ui.RGBA(255, 255, 255, 0.5)
	cardFill    = ui.RGBA(255, 255, 255, 0.065)
	cardStroke  = ui.RGBA(255, 255, 255, 0.08)
	panelTop    = ui.RGB(18, 19, 21)
	panelBottom = ui.RGB(10, 10, 10)
	darkTheme   = func() *ui.Theme {
		t := *ui.DarkTheme()
		t.Accent, t.AccentHover, t.AccentPressed = accent, accent, accentDeep
		return &t
	}()
)

func icon(c *ui.Context, name string) ui.Element { return ui.Icon(c, icons[name]) }

// card is the rounded, faintly lit box every group sits in.
func card(c *ui.Context, children func()) ui.Element {
	return ui.Column(c).Padding(12).Gap(10).Radius(12).Background(cardFill).Border(0.7, cardStroke).Children(children)
}

// chip is a pressable tile; selected ones take the accent.
func chip(c *ui.Context, selected bool) ui.Element {
	e := ui.Box(c).Radius(9).Center().Cursor(ui.CursorPointer)
	switch {
	case selected:
		e.Background(accent.Alpha(0.18)).Border(0.8, accent.Alpha(0.55)).TextColor(accent)
	case e.Pressed():
		e.Background(ui.RGBA(255, 255, 255, 0.13)).TextColor(ink)
	case e.Hovered():
		e.Background(ui.RGBA(255, 255, 255, 0.1)).TextColor(ink)
	default:
		e.Background(ui.RGBA(255, 255, 255, 0.06)).TextColor(ink)
	}
	return e
}

// chipText is a chip with one line of text, as the input and refresh-rate
// buttons.
func chipText(c *ui.Context, label string, selected bool) bool {
	e := chip(c, selected).Height(30)
	e.Children(func() { ui.Text(c, label).FontSize(11).FontWeight(500).SingleLine() })
	return e.Clicked()
}

// cardTitle heads a card: an icon, a title, and a note at the end.
func cardTitle(c *ui.Context, title, iconName, trailing string) {
	ui.Row(c).Gap(6).Children(func() {
		icon(c, iconName).FontSize(13)
		ui.Text(c, title).FontSize(12).FontWeight(600).Grow(1)
		if trailing != "" {
			ui.Text(c, trailing).FontSize(10).TextColor(muted)
		}
	})
}

func sectionTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(11).Bold().LetterSpacing(0.8).TextColor(muted).Padding(0, 1)
}

// sliderCard is a titled slider with − and + buttons and its value large.
type sliderCard struct {
	key, title, caption, icon string
	value                     *float64
	lo, hi, step              float64
	format                    func(float64) string // the value as shown, default an integer
}

// show draws it and reports whether the user changed the value.
func (s sliderCard) show(c *ui.Context) bool {
	changed := false
	card(c, func() {
		ui.Row(c).Gap(8).Children(func() {
			icon(c, s.icon).FontSize(14).TextColor(muted).Width(18)
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, s.title).FontSize(12).FontWeight(600)
				ui.Text(c, s.caption).FontSize(10).TextColor(muted)
			})
			text := fmt.Sprint(int(math.Round(*s.value)))
			if s.format != nil {
				text = s.format(*s.value)
			}
			ui.Text(c, text).FontSize(20).FontWeight(600).FontFeatures("tnum")
		})
		ui.Row(c).Gap(8).Children(func() {
			if stepButton(c, "minus") {
				*s.value, changed = max(*s.value-s.step, s.lo), true
			}
			if ui.Slider(c.Key(s.key), s.value, s.lo, s.hi).Label(s.title).Grow(1).Changed() {
				changed = true
			}
			if stepButton(c, "plus") {
				*s.value, changed = min(*s.value+s.step, s.hi), true
			}
		})
	})
	return changed
}

func stepButton(c *ui.Context, name string) bool {
	e := chip(c, false).Size(26, 22)
	e.Children(func() { icon(c, name).FontSize(11) })
	return e.Clicked()
}
