package main

import (
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
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
	sc := &screen{
		monitor: display.Monitor{ID: "mi", Name: "Mi Monitor", Connection: "HDMI"},
		link:    mitv.New("192.0.2.1"),
		inputs:  []mitv.Source{{ID: 23, Name: "HDMI1"}, {ID: 24, Name: "HDMI2"}, {ID: 29, Name: "DP"}, {ID: 30, Name: "USB-C"}},
		volume:  80, hasVolume: true, deviceVolume: 80,
		soft: 100,
	}
	s := &store{
		prefs:    &settings{SoftBrightness: 100, FollowMin: 10, FollowMax: 100},
		tracker:  usage.Open(filepath.Join(t.TempDir(), "usage.json")),
		redraw:   func() {},
		setGamma: func(display.Monitor, float64, float64) error { return nil },
		screens:  []*screen{sc},
		cur:      sc,
		now:      func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local) },
	}
	s.setGamma = func(_ display.Monitor, b, w float64) error { gamma = [2]float64{b, w}; return nil }
	s.prefs.DayScene, s.prefs.NightScene = "日间", "夜间"
	s.dayStart, s.nightStart = parseClock("07:00", 0), parseClock("19:00", 0)
	for _, rate := range []int{160, 144, 120, 60} {
		for _, w := range []int{3360, 3200, 2560, 1920, 1600, 1280} {
			sc.modes = append(sc.modes, display.Mode{Width: w, Height: w * 9 / 16, PixelWidth: 2 * w, PixelHeight: w * 9 / 8, RefreshRate: rate})
		}
		for _, w := range []int{3840, 2560, 1920} {
			sc.modes = append(sc.modes, display.Mode{Width: w, Height: w * 9 / 16, PixelWidth: w, PixelHeight: w * 9 / 16, RefreshRate: rate})
		}
	}
	sc.current, sc.hasCurrent = sc.modes[3], true // 1920 × 1080 HiDPI at 160 Hz
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
	p.s.cur.link, p.s.cur.inputs, p.s.cur.hasVolume = nil, nil, false
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
	s.cur.link, s.cur.inputs = nil, nil
	s.cur.hasBrightness, s.cur.brightness = true, 70
	s.cur.ddcInputs = []mitv.Source{{ID: 0x0F, Name: display.InputName(0x0F)}, {ID: 0x11, Name: display.InputName(0x11)}}
	s.cur.controls = []control{{code: display.VCPContrast, name: "对比度", value: 50, max: 100}}
	tt := newTester(p)
	for _, want := range []string{"输入源", "DDC/CI", "DP", "HDMI 1", "硬件背光 · DDC/CI", "70"} {
		if !tt.HasText(want) {
			t.Errorf("overview: missing %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("夜间"); err != nil {
		t.Fatal(err)
	}
	if s.cur.brightness != 30 || s.cur.soft != 100 {
		t.Errorf("night scene: backlight %v, software %v; want the backlight dimmed", s.cur.brightness, s.cur.soft)
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

// A newer version shows a bar offering it; none shows nothing.
func TestUpdateBar(t *testing.T) {
	p, _ := testPanel(t)
	tt := newTester(p)
	if tt.HasText("更新并重启") {
		t.Fatal("an update bar without an update")
	}
	p.s.update = &mygo.Update{Version: "9.9.9"}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("新版本 9.9.9") || !tt.HasText("更新并重启") {
		t.Errorf("no update bar in %q", tt.Texts())
	}
}

// Issue #2: a monitor reporting an input it doesn't list keeps the one
// chosen in the app highlighted; the header gives the current refresh rate.
func TestRedmi27NU(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	sc := s.cur
	sc.link, sc.inputs = nil, nil
	sc.monitor = display.Monitor{ID: "nu", Name: "Redmi 27 NU", Connection: "DP"}
	sc.ddcInputs = []mitv.Source{{ID: 0x11, Name: "HDMI 1"}, {ID: 0x12, Name: "HDMI 2"}, {ID: 0x0F, Name: "DP"}, {ID: 0x10, Name: "USB-C"}}
	sc.ddc = nil
	sc.input = 0x10
	s.applyInput(sc, ddcReading{values: map[byte][2]int{display.VCPInput: {8, 14}}})
	if sc.input != 0x10 {
		t.Errorf("input = %#x after reading 8, want the chosen 0x10 kept", sc.input)
	}
	s.applyInput(sc, ddcReading{values: map[byte][2]int{display.VCPInput: {0x11, 14}}})
	if sc.input != 0x11 {
		t.Errorf("input = %#x after reading 0x11, want 0x11", sc.input)
	}
	for _, m := range sc.modes {
		if m.Width == 1920 && m.HiDPI() && m.RefreshRate == 60 {
			sc.current = m
		}
	}
	tt := newTester(p)
	if !tt.HasText("外接显示器 · DP · 4K · 60Hz") {
		t.Errorf("header in %q, want the current 60Hz", tt.Texts())
	}
	if !tt.HasText("USB-C") {
		t.Errorf("no USB-C input in %q", tt.Texts())
	}
}

// Issue #3: the backlight follows the built-in display, easing over a few
// seconds, and keeps a manual change until the built-in display changes.
func TestFollowBuiltin(t *testing.T) {
	if got := followTarget(0.5, 10, 90); got != 50 {
		t.Errorf("followTarget(0.5, 10, 90) = %v, want 50", got)
	}
	p, _ := testPanel(t)
	s := p.s
	tt := newTester(p)
	if err := tt.Click("画面"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("跟随内屏亮度") {
		t.Fatal("the card shows without a built-in display and DDC/CI")
	}
	s.hasBuiltin, s.cur.hasBrightness, s.cur.brightness, s.lastBuiltin = true, true, 30, -1
	tt.Frame()
	if err := tt.Click("启用跟随内屏亮度"); err != nil {
		t.Fatalf("no follow switch: %v in %q", err, tt.Texts())
	}
	tt.Frame()
	builtin := 0.8
	read := func() (float64, bool) { return builtin, true }
	steps := 0
	for ; steps < 30 && (steps == 0 || s.easing); steps++ {
		before := s.cur.brightness
		s.followTick(read)
		if jump := math.Abs(s.cur.brightness - before); jump > 25 {
			t.Fatalf("step %d jumped %v points", steps, jump)
		}
	}
	if want := followTarget(0.8, s.prefs.FollowMin, s.prefs.FollowMax); s.cur.brightness != want {
		t.Fatalf("eased to %v after %d steps, want %v", s.cur.brightness, steps, want)
	}
	s.setBrightness(40) // a manual change
	for range 5 {
		s.followTick(read)
	}
	if s.cur.brightness != 40 {
		t.Errorf("following undid a manual change while the built-in display stayed: %v", s.cur.brightness)
	}
	builtin = 0.2
	for range 30 {
		s.followTick(read)
	}
	if want := followTarget(0.2, s.prefs.FollowMin, s.prefs.FollowMax); s.cur.brightness != want {
		t.Errorf("after the built-in display dimmed: %v, want %v", s.cur.brightness, want)
	}
}

// Each display keeps its own picture. Switching shows that display's mode.
func TestEachMonitorKeepsItsPicture(t *testing.T) {
	p, gamma := testPanel(t)
	s := p.s
	other := &screen{
		monitor:    display.Monitor{ID: "studio", Name: "Studio Display", Connection: "Thunderbolt"},
		soft:       100,
		current:    display.Mode{Width: 2560, Height: 1440, PixelWidth: 2560, PixelHeight: 1440, RefreshRate: 60},
		hasCurrent: true,
	}
	s.screens = []*screen{s.cur, other}
	tt := newTester(p)
	if !tt.HasText("Studio Display") {
		t.Fatalf("switcher missing in %q", tt.Texts())
	}
	if err := tt.Click("Studio Display"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.cur != other {
		t.Fatalf("selected %s", s.cur.monitor.Name)
	}
	if !tt.HasText("2560 × 1440") {
		t.Fatalf("studio mode missing in %q", tt.Texts())
	}
	if err := tt.Click("夜间"); err != nil {
		t.Fatal(err)
	}
	if other.soft != 30 || other.warmth != 55 || *gamma != [2]float64{0.3, 0.55} {
		t.Fatalf("studio picture soft %v warmth %v gamma %v", other.soft, other.warmth, *gamma)
	}
	if s.screens[0].soft != 100 || s.screens[0].warmth != 0 {
		t.Fatalf("first monitor changed: soft %v warmth %v", s.screens[0].soft, s.screens[0].warmth)
	}
	if err := tt.Click("Mi Monitor"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.cur.monitor.Name != "Mi Monitor" || !tt.HasText("1920 × 1080") {
		t.Fatalf("back on %s, texts %q", s.cur.monitor.Name, tt.Texts())
	}
}

// TestSnapshots writes each tab as a PNG to $MONITORBAR_SNAPSHOTS, to look at.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("MONITORBAR_SNAPSHOTS")
	if dir == "" {
		t.Skip("set MONITORBAR_SNAPSHOTS to a directory")
	}
	files := map[string]string{"概览": "overview", "画面": "picture", "分辨率": "modes", "屏幕时间": "activity"}
	for _, tb := range tabs {
		p, _ := testPanel(t)
		p.s.prefs.Tab = tb.name
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
	p.s.setGamma = func(_ display.Monitor, b, w float64) error {
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
