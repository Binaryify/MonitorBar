package main

import (
	"errors"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/locales"
)

func TestResolutionListsKeepHighestThree(t *testing.T) {
	p, _ := testPanel(t)
	p.s.prefs.Tab = "modes"
	p.s.link = nil
	for _, width := range []int{1600, 1280} {
		p.s.modes = append(p.s.modes, display.Mode{Width: width, Height: width * 9 / 16, PixelWidth: width, PixelHeight: width * 9 / 16, RefreshRate: 160})
	}
	tt := newTester(p)
	closedHeight := p.contentHeight
	for _, want := range []string{"HiDPI（清晰）", "标准", "3360 × 1890", "3200 × 1800", "2560 × 1440", "3840 × 2160", "显示其余 3 种", "显示其余 2 种"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	for _, hidden := range []string{"1600 × 900", "1280 × 720", "渲染 3840×2160"} {
		if tt.HasText(hidden) {
			t.Errorf("unexpected %q in the collapsed lists", hidden)
		}
	}
	if err := tt.Click("显示其余 3 种"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if !tt.HasText("渲染 3840×2160") || !tt.HasText("渲染 2560×1440") || !p.s.hiDPIModesOpen || p.s.standardModesOpen {
		t.Fatal("show more must extend only the HiDPI list")
	}
	tt.Key(0, ui.KeySpace)
	for range 3 {
		tt.Frame()
	}
	if p.s.hiDPIModesOpen || tt.HasText("渲染 3840×2160") || p.contentHeight != closedHeight {
		t.Fatal("Space must restore the original three rows and their height")
	}
	tt.Key(0, ui.KeyEnter)
	for range 3 {
		tt.Frame()
	}
	if !p.s.hiDPIModesOpen || !tt.HasText("渲染 3840×2160") {
		t.Fatal("Enter must extend the same list")
	}
	tt.Key(0, ui.KeySpace)
	for range 3 {
		tt.Frame()
	}
	if err := tt.Click("显示其余 2 种"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		tt.Frame()
	}
	if !p.s.standardModesOpen || p.s.hiDPIModesOpen || !tt.HasText("1280 × 720") || tt.HasText("渲染 2560×1440") {
		t.Fatal("the standard list must extend independently")
	}
	p.s.modeSwitch = p.s.output
	tt.Frame()
	if err := tt.Click("3360 × 1890"); err != nil {
		t.Fatal(err)
	}
	if p.s.notice != "" {
		t.Fatal("disabled resolution must not start a display operation")
	}
}

func TestShortResolutionListsNeedNoDisclosure(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3} {
		t.Run(strconv.Itoa(count)+" modes", func(t *testing.T) {
			p, _ := testPanel(t)
			p.s.prefs.Tab = "modes"
			p.s.link = nil
			p.s.modes = p.s.modes[:count]
			tt := newTester(p)
			if tt.HasText("显示其余") || tt.HasText("收起") {
				t.Fatal("lists with at most three sizes must not offer show more")
			}
			for _, mode := range p.s.modes {
				if !tt.HasText(mode.Title()) {
					t.Errorf("missing available mode %v", mode)
				}
			}
		})
	}
}

func TestDisplaysKeepIndependentResolutionLists(t *testing.T) {
	p, _ := twoDisplays(t)
	s := p.s
	s.prefs.Tab = "modes"
	a, b := s.outputs[0], s.outputs[1]
	a.link = nil
	b.modes = append([]display.Mode(nil), a.modes...)
	for _, width := range []int{1600, 1280} {
		b.modes = append(b.modes, display.Mode{Width: width, Height: width * 9 / 16, PixelWidth: width, PixelHeight: width * 9 / 16, RefreshRate: 160})
	}
	b.current, b.hasCurrent = a.current, true
	tt := newTester(p)
	if err := tt.Click("显示其余 3 种"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.hiDPIModesOpen || !tt.HasText("渲染 2560×1440") {
		t.Fatal("display A did not expand")
	}
	s.showDisplay(b)
	tt.Frame()
	if b.hiDPIModesOpen || tt.HasText("渲染 2560×1440") {
		t.Fatal("display B inherited A's expansion")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if b.hiDPIModesOpen || b.standardModesOpen {
		t.Fatal("display B inherited A's keyboard focus")
	}
	if err := tt.Click("显示其余 2 种"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !b.standardModesOpen || b.hiDPIModesOpen || !tt.HasText("1280 × 720") {
		t.Fatal("display B did not expand independently")
	}
	s.showDisplay(a)
	tt.Frame()
	if !a.hiDPIModesOpen || a.standardModesOpen || !tt.HasText("渲染 2560×1440") {
		t.Fatal("display A lost its expansion")
	}
	s.modeSwitch = a
	s.prefs.SyncDisplays = true
	s.showDisplay(b)
	tt.Frame()
	for _, label := range []string{"3360 × 1890", "144 Hz"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if s.modeSwitch != a || s.notice != "" {
		t.Fatal("another display started a concurrent mode switch")
	}
}

func TestModeCompletionBelongsToItsDisplay(t *testing.T) {
	for _, name := range []string{"selected", "failed", "other selected", "removed", "replacement", "identity changed"} {
		t.Run(name, func(t *testing.T) {
			p, _ := twoDisplays(t)
			s := p.s
			a, b := s.outputs[0], s.outputs[1]
			monitor, want := a.monitor, a.current
			s.modeSwitch, s.notice = a, "pending"
			var err error
			switch name {
			case "failed":
				err = errors.New("rejected")
			case "other selected":
				s.showDisplay(b)
				s.notice = "display B notice"
			case "removed":
				s.outputs = []*output{b}
				s.output = b
				s.notice = "display B notice"
			case "replacement":
				s.output = &output{monitor: monitor}
				s.outputs = []*output{s.output, b}
				s.notice = "replacement notice"
			case "identity changed":
				a.monitor.Name = "replacement"
				s.notice = "replacement notice"
			}
			selected, notice := s.output, s.notice
			s.modeApplied(a, monitor, want, err)
			if s.modeSwitch != nil || s.output != selected {
				t.Fatal("completion did not clear its operation or changed selection")
			}
			switch name {
			case "selected":
				if s.notice != locales.NoticeModeSwitched(want.Title(), want.RefreshRate) {
					t.Fatalf("success notice: %q", s.notice)
				}
			case "failed":
				if s.notice != locales.NoticeModeRejected() {
					t.Fatalf("failure notice: %q", s.notice)
				}
			default:
				if s.notice != notice {
					t.Fatalf("stale completion changed notice %q to %q", notice, s.notice)
				}
			}
		})
	}
}
