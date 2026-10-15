package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/usage"
)

// testPanel is the panel over a store of fixed data, connected to the
// display over the LAN, without touching the system.
func testPanel(t *testing.T) (*panel, *[2]float64) {
	t.Helper()
	var gamma [2]float64
	s := &store{
		prefs:    &settings{SoftBrightness: 100},
		tracker:  usage.Open(filepath.Join(t.TempDir(), "usage.json")),
		redraw:   func() {},
		setGamma: func(b, w float64) error { gamma = [2]float64{b, w}; return nil },
		monitor:  display.Monitor{Name: "Mi Monitor", Connection: "HDMI"},
		link:     mitv.New("192.0.2.1"),
		inputs:   []mitv.Source{{ID: 23, Name: "HDMI1"}, {ID: 24, Name: "HDMI2"}, {ID: 29, Name: "DP"}, {ID: 30, Name: "USB-C"}},
		volume:   80, hasVolume: true, deviceVolume: 80,
		now: func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local) },
	}
	s.prefs.DayScene, s.prefs.NightScene = "日间", "夜间"
	s.dayStart, s.nightStart = parseClock("07:00", 0), parseClock("19:00", 0)
	for _, rate := range []int{160, 144, 120, 60} {
		for _, w := range []int{3360, 3200, 2560, 1920, 1600, 1280} {
			s.modes = append(s.modes, display.Mode{Width: w, Height: w * 9 / 16, PixelWidth: 2 * w, PixelHeight: w * 9 / 8, RefreshRate: rate})
		}
		for _, w := range []int{3840, 2560, 1920} {
			s.modes = append(s.modes, display.Mode{Width: w, Height: w * 9 / 16, PixelWidth: w, PixelHeight: w * 9 / 16, RefreshRate: rate})
		}
	}
	s.current, s.hasCurrent = s.modes[3], true // 1920 × 1080 HiDPI at 160 Hz
	day := func(d string, awake, active int) usage.Day { return usage.Day{Day: d, Awake: awake, Active: active} }
	s.usage = []usage.Day{day("2026-10-08", 5*3600, 3*3600), day("2026-10-09", 8*3600, 6*3600), day("2026-10-10", 2*3600, 3000),
		day("2026-10-11", 0, 0), day("2026-10-12", 6*3600, 4*3600), day("2026-10-13", 9*3600, 7*3600), day("2026-10-14", 3*3600, 2*3600)}
	return &panel{s: s, maxContent: 640, arrowX: panelWidth / 2}, &gamma
}

func newTester(p *panel) *ui.Tester {
	tt := ui.NewTester(p.view, panelWidth, 1100)
	for range 3 { // the scroll area takes its content's height a frame later
		tt.Frame()
	}
	return tt
}

func TestOverview(t *testing.T) {
	p, _ := testPanel(t)
	tt := newTester(p)
	for _, want := range []string{"Mi Monitor", "外接显示器 · HDMI · 4K · 160Hz", "1920 × 1080", "HDMI1", "DP", "USB-C", "显示器音量", "已通过局域网连接 · 澎湃 OS 遥控"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("硬件控制未就绪") {
		t.Error("the notice shows while connected")
	}
}

func TestSceneSetsGamma(t *testing.T) {
	p, gamma := testPanel(t)
	tt := newTester(p)
	if err := tt.Click("夜间"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if p.s.prefs.SoftBrightness != 30 || p.s.prefs.Warmth != 55 || *gamma != [2]float64{0.3, 0.55} {
		t.Errorf("brightness %v warmth %v gamma %v", p.s.prefs.SoftBrightness, p.s.prefs.Warmth, *gamma)
	}
}

func TestTabs(t *testing.T) {
	p, _ := testPanel(t)
	tt := newTester(p)
	for tab, want := range map[string][]string{
		"画面":   {"软件调光", "色温", "标准", "恢复软件调整"},
		"分辨率":  {"刷新率", "160 Hz", "HiDPI（清晰）", "渲染 3840×2160", "标准"},
		"屏幕时间": {"今日亮屏", "3 小时 0 分", "最近 7 天", "日均 5 小时 30 分"},
	} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		for _, w := range want {
			if !tt.HasText(w) {
				t.Errorf("tab %s: missing %q in %q", tab, w, tt.Texts())
			}
		}
	}
}

// With no channel to the monitor, the panel shows what works and says
// nothing about what doesn't.
func TestNoChannelShowsNoUnsupportedText(t *testing.T) {
	p, _ := testPanel(t)
	p.s.link, p.s.inputs, p.s.hasVolume = nil, nil, false
	tt := newTester(p)
	for _, tab := range []string{"概览", "画面"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		for _, text := range tt.Texts() {
			for _, word := range []string{"不支持", "未找到", "未就绪", "需要显示器"} {
				if strings.Contains(text, word) {
					t.Errorf("tab %s shows %q", tab, text)
				}
			}
		}
		if tt.HasText("输入源") || tt.HasText("显示器音量") || tt.HasText("已通过局域网连接 · 澎湃 OS 遥控") {
			t.Errorf("tab %s shows controls of a missing channel: %q", tab, tt.Texts())
		}
	}
	if !tt.HasText("软件调光") {
		t.Error("software dimming is missing")
	}
}

// A monitor reached over DDC/CI alone gets its inputs, its backlight and
// the picture controls it answers.
func TestDDCOnly(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link, s.inputs = nil, nil
	s.hasBrightness, s.brightness = true, 70
	s.ddcInputs = []mitv.Source{{ID: 0x0F, Name: display.InputName(0x0F)}, {ID: 0x11, Name: display.InputName(0x11)}}
	s.controls = []control{{code: display.VCPContrast, name: "对比度", value: 50, max: 100}}
	tt := newTester(p)
	for _, want := range []string{"输入源", "DDC/CI", "DP", "HDMI 1", "硬件背光 · DDC/CI", "70"} {
		if !tt.HasText(want) {
			t.Errorf("overview: missing %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("夜间"); err != nil {
		t.Fatal(err)
	}
	if s.brightness != 30 || s.prefs.SoftBrightness != 100 {
		t.Errorf("night scene: backlight %v, software %v; want the backlight dimmed", s.brightness, s.prefs.SoftBrightness)
	}
	if err := tt.Click("画面"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("对比度") || !tt.HasText("硬件 · DDC/CI") {
		t.Errorf("picture: missing the contrast control in %q", tt.Texts())
	}
}

// Turning the switch on applies the current period's scene; a manual
// change stays until the next switch, which applies the other scene.
func TestAutoSwitch(t *testing.T) {
	p, gamma := testPanel(t)
	s := p.s
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	s.now = func() time.Time { return now }
	tt := newTester(p)
	if err := tt.Click("画面"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("启用自动切换"); err != nil { // the switch, by its label
		t.Fatal(err)
	}
	tt.Frame()
	if !s.prefs.AutoSwitch || s.prefs.SoftBrightness != 80 || s.prefs.AutoPeriod != "day" {
		t.Fatalf("after turning on at noon: on %v, brightness %v, period %q", s.prefs.AutoSwitch, s.prefs.SoftBrightness, s.prefs.AutoPeriod)
	}
	if !tt.HasText("现在是白天，19:00 切换到「夜间」") {
		t.Errorf("status missing in %q", tt.Texts())
	}

	s.setSoftBrightness(65) // a manual change
	now = now.Add(time.Hour)
	s.autoCheck()
	if s.prefs.SoftBrightness != 65 {
		t.Errorf("a check within the day undid the manual change: %v", s.prefs.SoftBrightness)
	}

	now = time.Date(2026, 10, 15, 19, 0, 30, 0, time.Local)
	s.autoCheck()
	if s.prefs.SoftBrightness != 30 || s.prefs.Warmth != 55 || *gamma != [2]float64{0.3, 0.55} {
		t.Errorf("at 19:00: brightness %v warmth %v gamma %v, want the night scene", s.prefs.SoftBrightness, s.prefs.Warmth, *gamma)
	}
}

// TestSnapshots writes each tab as a PNG to $MONITORBAR_SNAPSHOTS, to look at.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("MONITORBAR_SNAPSHOTS")
	if dir == "" {
		t.Skip("set MONITORBAR_SNAPSHOTS to a directory")
	}
	files := map[string]string{"概览": "overview", "共享": "sharing", "画面": "picture", "分辨率": "modes", "屏幕时间": "activity"}
	for _, tb := range tabs {
		p, _ := testPanel(t)
		p.s.prefs.Tab = tb.name
		if tb.name == "共享" {
			seedShare(p)
		}
		tt := newTester(p)
		tt.SetScale(2)
		tt.Frame()
		img := tt.Image()
		crop := img.SubImage(image.Rect(0, 0, img.Bounds().Dx(), p.height*2))
		f, err := os.Create(filepath.Join(dir, files[tb.name]+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, crop); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

// The panel must report its natural height even while its window is
// shorter, or it could never grow to fit a notice.
func TestPanelReportsNaturalHeight(t *testing.T) {
	p, _ := testPanel(t)
	tt := ui.NewTester(p.view, panelWidth, 600)
	for range 6 {
		tt.Frame()
	}
	natural := p.height
	t.Logf("natural height in a 600 DIP window: %d", natural)
	if natural <= 600 {
		t.Fatalf("height %d is clamped to the window", natural)
	}
	p.s.notice = "一条提示，让内容变高。"
	for range 6 {
		tt.Frame()
	}
	if p.height <= natural {
		t.Fatalf("height %d did not grow past %d with a notice", p.height, natural)
	}
}

// A notice that appears after a click must grow the panel by itself, with
// no other frame asked for: windows draw only on demand.
func TestNoticeGrowsPanelWithoutInput(t *testing.T) {
	p, _ := testPanel(t)
	p.s.setGamma = func(b, w float64) error {
		if b < 0.5 {
			return display.ErrGammaLimited
		}
		return nil
	}
	tt := newTester(p)
	before := p.height
	if err := tt.Click("夜间"); err != nil { // Click draws only the frames the view asks for
		t.Fatal(err)
	}
	if !tt.HasText(gammaLimited) {
		t.Fatalf("no notice in %q", tt.Texts())
	}
	if p.height <= before {
		t.Fatalf("height stayed %d after the notice appeared", p.height)
	}
}
