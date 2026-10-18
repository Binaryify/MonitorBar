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
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mediakeys"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/usage"
	"github.com/binaryify/MonitorBar/locales"
)

// The tests read the panel in Simplified Chinese, its first language,
// unless they choose another.
func TestMain(m *testing.M) {
	i18n.SetLocaleFunc(func() string { return testLocale })
	os.Exit(m.Run())
}

var testLocale = "zh-Hans"

// testPanel is the panel over a store of fixed data, connected to the
// display over the LAN, without touching the system.
func testPanel(t *testing.T) (*panel, *[2]float64) {
	t.Helper()
	var gamma [2]float64
	s := &store{
		prefs:    &settings{SoftBrightness: 100, FollowMin: 10, FollowMax: 100},
		tracker:  usage.Open(filepath.Join(t.TempDir(), "usage.json")),
		redraw:   func() {},
		setGamma: func(b, w float64) error { gamma = [2]float64{b, w}; return nil },
		monitor:  display.Monitor{Name: "Mi Monitor", Connection: "HDMI"},
		link:     mitv.New("192.0.2.1"),
		inputs:   []mitv.Source{{ID: 23, Name: "HDMI1"}, {ID: 24, Name: "HDMI2"}, {ID: 29, Name: "DP"}, {ID: 30, Name: "USB-C"}},
		volume:   80, hasVolume: true, deviceVolume: 80,
		soundToDisplay: func() bool { return true },
		now:            func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local) },
	}
	s.prefs.DayScene, s.prefs.NightScene = "day", "night"
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
	tt.SetLocale(testLocale) // MyGo's own controls
	for range 3 {            // the scroll area takes its content's height a frame later
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
		"画面":   {"色温", "标准", "恢复软件调整"},
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
	if err := tt.Click("概览"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("软件调光 · Gamma") {
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
	s.controls = []control{{code: display.VCPContrast, label: locales.ControlContrast, value: 50, max: 100}}
	tt := newTester(p)
	for _, want := range []string{"输入源", "DDC/CI", "DP", "HDMI 1", "组合 · 硬件背光 + 软件调光", "85"} {
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
	s.link, s.inputs = nil, nil
	s.monitor = display.Monitor{Name: "Redmi 27 NU", Connection: "DP"}
	s.ddcInputs = []mitv.Source{{ID: 0x11, Name: "HDMI 1"}, {ID: 0x12, Name: "HDMI 2"}, {ID: 0x0F, Name: "DP"}, {ID: 0x10, Name: "USB-C"}}
	s.ddc = nil
	s.input = 0x10
	s.applyInput(ddcReading{values: map[byte][2]int{display.VCPInput: {8, 14}}})
	if s.input != 0x10 {
		t.Errorf("input = %#x after reading 8, want the chosen 0x10 kept", s.input)
	}
	s.applyInput(ddcReading{values: map[byte][2]int{display.VCPInput: {0x11, 14}}})
	if s.input != 0x11 {
		t.Errorf("input = %#x after reading 0x11, want 0x11", s.input)
	}
	for _, m := range s.modes {
		if m.Width == 1920 && m.HiDPI() && m.RefreshRate == 60 {
			s.current = m
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
	s.hasBuiltin, s.hasBrightness, s.brightness, s.lastBuiltin = true, true, 30, -1
	tt.Frame()
	if err := tt.Click("启用跟随内屏亮度"); err != nil {
		t.Fatalf("no follow switch: %v in %q", err, tt.Texts())
	}
	tt.Frame()
	builtin := 0.8
	read := func() (float64, bool) { return builtin, true }
	steps := 0
	for ; steps < 30 && (steps == 0 || s.easing); steps++ {
		before := s.brightness
		s.followTick(read)
		if jump := math.Abs(s.brightness - before); jump > 25 {
			t.Fatalf("step %d jumped %v points", steps, jump)
		}
	}
	if want := followTarget(0.8, s.prefs.FollowMin, s.prefs.FollowMax); s.brightness != want {
		t.Fatalf("eased to %v after %d steps, want %v", s.brightness, steps, want)
	}
	s.setBrightness(40) // a manual change
	for range 5 {
		s.followTick(read)
	}
	if s.brightness != 40 {
		t.Errorf("following undid a manual change while the built-in display stayed: %v", s.brightness)
	}
	builtin = 0.2
	for range 30 {
		s.followTick(read)
	}
	if want := followTarget(0.2, s.prefs.FollowMin, s.prefs.FollowMax); s.brightness != want {
		t.Errorf("after the built-in display dimmed: %v, want %v", s.brightness, want)
	}
}

// TestSnapshots writes each tab as a PNG to $MONITORBAR_SNAPSHOTS, to look at.
func TestSnapshots(t *testing.T) {
	dir := os.Getenv("MONITORBAR_SNAPSHOTS")
	if dir == "" {
		t.Skip("set MONITORBAR_SNAPSHOTS to a directory")
	}
	for _, tb := range tabs {
		p, _ := testPanel(t)
		p.s.prefs.Tab = tb.id
		tt := newTester(p)
		tt.SetScale(2)
		tt.Frame()
		img := tt.Image()
		crop := img.SubImage(image.Rect(0, 0, img.Bounds().Dx(), p.height*2))
		f, err := os.Create(filepath.Join(dir, tb.id+".png"))
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
	if !tt.HasText(locales.NoticeGammaLimited()) {
		t.Fatalf("no notice in %q", tt.Texts())
	}
	if p.height <= before {
		t.Fatalf("height stayed %d after the notice appeared", p.height)
	}
}

// The combined brightness puts the backlight above the switchover and
// software dimming below it, both ways (#9).
func TestCombinedBrightness(t *testing.T) {
	for _, c := range []struct{ combined, hardware, soft float64 }{
		{100, 100, 100},
		{75, 50, 100},
		{50, 0, 100},
		{25, 0, 60},
		{0, 0, softMin},
	} {
		if h, s := split(c.combined); h != c.hardware || s != c.soft {
			t.Errorf("split(%v) = %v, %v; want %v, %v", c.combined, h, s, c.hardware, c.soft)
		}
		if got := combine(c.hardware, c.soft); got != c.combined {
			t.Errorf("combine(%v, %v) = %v; want %v", c.hardware, c.soft, got, c.combined)
		}
	}

	p, gamma := testPanel(t)
	s := p.s
	s.hasBrightness, s.brightness = true, 70
	s.stepBrightness(-60) // from 85 to 25: the backlight at its minimum, and software dimming
	if s.brightness != 0 || s.prefs.SoftBrightness != 60 || gamma[0] != 0.6 {
		t.Errorf("at 25: backlight %v, software %v, gamma %v", s.brightness, s.prefs.SoftBrightness, *gamma)
	}
	s.stepBrightness(35) // to 60: the software dimming lifts
	if s.brightness != 20 || s.prefs.SoftBrightness != 100 {
		t.Errorf("at 60: backlight %v, software %v", s.brightness, s.prefs.SoftBrightness)
	}
}

// Every tab is as tall as the overview, so the panel keeps its height and
// long tabs scroll (#9).
func TestTabsShareTheOverviewsHeight(t *testing.T) {
	p, _ := testPanel(t)
	p.maxContent = 2000
	tt := newTester(p)
	want := p.height
	for _, tab := range []string{"画面", "分辨率", "屏幕时间", "概览"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			tt.Frame()
		}
		if p.height != want {
			t.Errorf("tab %s: panel %d high, want the overview's %d", tab, p.height, want)
		}
	}
}

// A shortcut records the keys pressed with Ctrl, Option or Command, or a
// lone function key; Esc
// keeps the old ones and Delete clears them (#9).
func TestRecordShortcut(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	tt := newTester(p)
	if err := tt.Click("画面"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(keysLabel("Ctrl+Alt+Up")) {
		t.Fatalf("the default brightness shortcut is missing in %q", tt.Texts())
	}
	record := func() {
		t.Helper()
		if err := tt.Click("亮度 +快捷键"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if s.recording != "brightnessUp" {
			t.Fatalf("recording %q after the click", s.recording)
		}
	}
	record()
	tt.Key(ui.Shift, ui.KeyB) // Shift alone would take B from every app
	if s.recording == "" {
		t.Error("Shift+B ended the recording")
	}
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyB)
	tt.Frame()
	if got := s.prefs.keys(shortcuts[0]); got != "Ctrl+Alt+B" || s.recording != "" {
		t.Errorf("after Ctrl+Alt+B: keys %q, recording %q", got, s.recording)
	}
	record()
	tt.Key(0, ui.KeyF2) // a function key types nothing, so it needs no modifier
	tt.Frame()
	if got := s.prefs.keys(shortcuts[0]); got != "F2" || s.recording != "" {
		t.Errorf("after F2: keys %q, recording %q", got, s.recording)
	}
	record()
	tt.Key(0, ui.KeyEscape)
	if got := s.prefs.keys(shortcuts[0]); got != "F2" || s.recording != "" {
		t.Errorf("after Esc: keys %q, recording %q", got, s.recording)
	}
	record()
	tt.Key(0, ui.KeyDelete)
	tt.Frame()
	if got := s.prefs.keys(shortcuts[0]); got != "" || !tt.HasText("未设置") {
		t.Errorf("after Delete: keys %q in %q", got, tt.Texts())
	}
}

// The media keys step the external display; macOS keeps the keys where the
// display can't take them, and a MacBook's own display shares the
// brightness keys.
func TestMediaKey(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil // DDC/CI
	s.hasBrightness, s.brightness = true, 50

	step, pass := s.mediaKey(mediakeys.BrightnessUp)
	if step == nil || pass {
		t.Fatalf("brightness up: step %v, pass %v", step != nil, pass)
	}
	before := s.combinedBrightness()
	step()
	if got := s.combinedBrightness(); got != before+shortcutStep {
		t.Errorf("brightness %v after up from %v", got, before)
	}
	if step, _ := s.mediaKey(mediakeys.VolumeDown); step == nil {
		t.Fatal("volume down left to macOS with the sound on the display")
	} else if step(); s.volume != 75 {
		t.Errorf("volume %v after down from 80", s.volume)
	}

	s.hasBuiltin = true
	if _, pass := s.mediaKey(mediakeys.BrightnessDown); !pass {
		t.Error("the built-in display lost its brightness keys")
	}
	s.prefs.FollowBuiltin = true
	if step, _ := s.mediaKey(mediakeys.BrightnessDown); step != nil {
		t.Error("a key stepped the backlight that follows the built-in display")
	}
	s.prefs.FollowBuiltin, s.monitor.Builtin = false, true
	if step, _ := s.mediaKey(mediakeys.BrightnessUp); step != nil {
		t.Error("a key stepped the built-in display, with no external one")
	}
	s.soundToDisplay = func() bool { return false }
	if step, _ := s.mediaKey(mediakeys.VolumeUp); step != nil {
		t.Error("a key took the volume of sound playing elsewhere")
	}
}

func TestStepVolume(t *testing.T) {
	p, _ := testPanel(t)
	s := p.s
	s.link = nil // DDC/CI, without the LAN remote's key presses
	s.stepVolume(5)
	if s.volume != 85 {
		t.Errorf("volume %v after +5 from 80", s.volume)
	}
	s.volume = 98
	s.stepVolume(5)
	if s.volume != 100 {
		t.Errorf("volume %v after +5 from 98", s.volume)
	}
}

// In English every tab says nothing in Chinese: a string left out of the
// catalogs would show.
func TestEnglish(t *testing.T) {
	testLocale = "en"
	defer func() { testLocale = "zh-Hans" }()
	for _, tb := range tabs {
		p, _ := testPanel(t)
		p.s.prefs.Tab, p.s.prefs.AutoSwitch = tb.id, true
		p.s.hasBrightness, p.s.hasBuiltin, p.s.prefs.FollowBuiltin = true, true, true
		tt := newTester(p)
		for _, text := range tt.Texts() {
			if strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
				t.Errorf("tab %s: %q is in Chinese", tb.id, text)
			}
		}
		if !tt.HasText(tb.label()) || !tt.HasText("External display · HDMI · 4K · 160Hz") {
			t.Errorf("tab %s: missing the English title or caption in %q", tb.id, tt.Texts())
		}
	}
}
