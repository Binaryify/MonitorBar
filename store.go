package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/schedule"
	"github.com/binaryify/MonitorBar/internal/usage"
)

// scene sets brightness and warmth at once.
type scene struct {
	name, icon         string
	brightness, warmth float64
}

var scenes = []scene{
	{"日间", "sun", 80, 0},
	{"阅读", "book", 55, 25},
	{"观影", "film", 100, 5},
	{"夜间", "moon", 30, 55},
}

// store holds what the panel shows and the actions that change it. Its
// fields are read and written on the main thread only: network work runs in
// goroutines and hands its results back through post.
type store struct {
	prefs    *settings
	tracker  *usage.Tracker
	redraw   func()                                 // asks the open panel for a new frame
	setGamma func(brightness, warmth float64) error // the monitor's, replaced in tests

	monitor    display.Monitor
	modes      []display.Mode
	current    display.Mode
	hasCurrent bool

	link      *mitv.Link // the display's LAN remote, nil until found
	detecting bool
	inputs    []mitv.Source
	input     int // the input chosen in the app, 0 before any
	volume    float64
	hasVolume bool

	usage         []usage.Day
	launchAtLogin bool
	notice        string
	dirty         bool // prefs changed since the last save

	volumeTarget atomic.Int64
	deviceVolume int // the display's volume as last known
	chasing      bool

	// DDC/CI, when the monitor answers it: hardware brightness, inputs and
	// volume (when the LAN remote has none), and picture controls.
	ddc           *display.DDC
	probingDDC    bool
	brightness    float64 // hardware %, meaningful when hasBrightness
	hasBrightness bool
	ddcInputs     []mitv.Source
	ddcVolume     bool
	controls      []control
	maxima        map[byte]int // the range of each VCP code

	// The day and night start times as the time inputs edit them.
	dayStart, nightStart time.Time
	now                  func() time.Time // the clock, fixed in tests

	// Updates, for builds that can update themselves.
	canUpdate  bool
	update     *mygo.Update // a newer version, nil when up to date
	installing bool
	progress   int // percent downloaded

	// Following the built-in display: whether there is one, its brightness
	// last seen (-1 before any), and the backlight being eased toward.
	hasBuiltin   bool
	lastBuiltin  float64
	followTarget float64
	easing       bool
}

// control is a picture setting the monitor offers over DDC/CI.
type control struct {
	code  byte
	name  string
	value float64
	max   int
}

// controlCatalog lists the settings shown when the monitor answers them.
var controlCatalog = []struct {
	code byte
	name string
}{
	{display.VCPContrast, "对比度"}, {display.VCPSharpness, "锐度"},
	{display.VCPRedGain, "红色增益"}, {display.VCPGreenGain, "绿色增益"}, {display.VCPBlueGain, "蓝色增益"},
}

func newStore() *store {
	s := &store{
		prefs:         loadSettings(),
		tracker:       usage.Open(filepath.Join(dataDir(), "usage.json")),
		redraw:        func() {},
		now:           time.Now,
		launchAtLogin: mygo.App.OpenAtLogin(),
		canUpdate:     mygo.Updater.Enabled(),
		lastBuiltin:   -1,
	}
	s.dayStart, s.nightStart = parseClock(s.prefs.DayStart, 7*60), parseClock(s.prefs.NightStart, 19*60)
	s.refreshDisplay()
	s.setGamma = func(b, w float64) error { return s.monitor.SetGamma(b, w) }
	s.applyGamma()
	s.usage = s.tracker.Recent(7)
	return s
}

// post runs fn on the main thread, then redraws.
func (s *store) post(fn func()) {
	mygo.RunOnMain(func() {
		fn()
		s.redraw()
	})
}

// MARK: Picture (software, through the gamma table)

func (s *store) setSoftBrightness(percent float64) {
	s.prefs.SoftBrightness = percent
	s.applyGamma()
}

func (s *store) setWarmth(percent float64) {
	s.prefs.Warmth = percent
	s.applyGamma()
}

// apply sets a scene's brightness, on the backlight when DDC/CI reaches it,
// and its warmth.
func (s *store) apply(sc scene) {
	if s.hasBrightness {
		s.setBrightness(sc.brightness)
	} else {
		s.prefs.SoftBrightness = sc.brightness
	}
	s.prefs.Warmth = sc.warmth
	s.applyGamma()
}

func (s *store) isActive(sc scene) bool {
	return math.Abs(s.effectiveBrightness()-sc.brightness) < 1 && math.Abs(s.prefs.Warmth-sc.warmth) < 1
}

// effectiveBrightness is the backlight's when DDC/CI reaches it, else the
// software dimming's.
func (s *store) effectiveBrightness() float64 {
	if s.hasBrightness {
		return s.brightness
	}
	return s.prefs.SoftBrightness
}

func (s *store) resetSoftware() {
	s.prefs.SoftBrightness, s.prefs.Warmth = 100, 0
	s.applyGamma()
}

const (
	gammaLimited = "Windows 限制了软件调光的幅度，已调到它允许的最暗。"
	gammaFailed  = "调光失败：系统拒绝了这个设置。"
)

func (s *store) applyGamma() {
	s.dirty = true
	err := s.setGamma(s.prefs.SoftBrightness/100, s.prefs.Warmth/100)
	switch {
	case errors.Is(err, display.ErrGammaLimited):
		s.notice = gammaLimited
	case err != nil:
		s.notice = gammaFailed
	case s.notice == gammaLimited || s.notice == gammaFailed:
		s.notice = ""
	}
}

// MARK: Network (HyperOS)

// sources are the inputs to switch between: the LAN remote's, else DDC/CI's.
func (s *store) sources() []mitv.Source {
	if s.link != nil {
		return s.inputs
	}
	return s.ddcInputs
}

// inputChannel names how inputs are switched, for the inputs card.
func (s *store) inputChannel() string {
	if s.link != nil {
		return "局域网"
	}
	return "DDC/CI"
}

// connect finds the display on the LAN: the host found last time first,
// then a scan.
func (s *store) connect() {
	if s.detecting {
		return
	}
	s.detecting = true
	cached := s.prefs.NetworkHost
	go func() {
		link := mitv.New(cached)
		// The host found last time must still be a monitor: addresses move.
		if cached == "" || !link.IsMonitor() {
			link = nil
			if host, err := mitv.Discover(); err == nil {
				link = mitv.New(host)
			}
		}
		var sources []mitv.Source
		volume, volumeErr := 0, error(nil)
		if link != nil {
			var err error
			if sources, err = link.Sources(); err != nil || len(sources) == 0 {
				link = nil
			} else {
				volume, volumeErr = link.Volume()
			}
		}
		s.post(func() {
			s.detecting = false
			s.link = link
			if link == nil {
				s.inputs, s.hasVolume = nil, s.ddcVolume // DDC/CI may still have volume
				return
			}
			s.prefs.NetworkHost, s.dirty = link.Host, true
			slices.SortFunc(sources, func(a, b mitv.Source) int { return a.ID - b.ID })
			s.inputs = sources
			if volumeErr == nil {
				s.deviceVolume, s.volume, s.hasVolume = volume, float64(volume), true
			}
		})
	}()
}

func (s *store) selectInput(src mitv.Source) {
	s.input = src.ID
	link := s.link
	if link == nil {
		if s.ddc != nil {
			s.ddc.Set(display.VCPInput, src.ID)
		}
		return
	}
	go func() {
		if err := link.Select(src.ID); err != nil {
			s.post(func() { s.notice = "切换到 " + src.Name + " 失败：显示器没有响应" })
		}
	}()
}

func (s *store) setVolume(percent float64) {
	s.volume = percent
	if s.link == nil {
		s.setDDC(display.VCPVolume, percent)
		return
	}
	s.volumeTarget.Store(int64(math.Round(percent)))
	s.chaseVolume()
}

// chaseVolume steps toward the latest target: the API only has ±1 keys.
func (s *store) chaseVolume() {
	if s.link == nil || s.chasing {
		return
	}
	s.chasing = true
	link, actual := s.link, s.deviceVolume
	go func() {
		failed := false
		for target := int(s.volumeTarget.Load()); target != actual; target = int(s.volumeTarget.Load()) {
			key, step := "volumeup", 1
			if target < actual {
				key, step = "volumedown", -1
			}
			if link.Press(key) != nil {
				failed = true
				break
			}
			actual += step
		}
		s.post(func() {
			s.chasing = false
			s.deviceVolume = actual
			switch {
			case failed:
				s.notice = "调节音量失败：显示器没有响应"
			case int(s.volumeTarget.Load()) != actual:
				s.chaseVolume() // the slider moved while the last step was sent
			default:
				s.readVolume()
			}
		})
	}()
}

func (s *store) readVolume() {
	link := s.link
	if link == nil {
		return
	}
	go func() {
		v, err := link.Volume()
		if err != nil {
			return
		}
		s.post(func() {
			s.deviceVolume, s.hasVolume = v, true
			if !s.chasing {
				s.volume = float64(v)
			}
		})
	}()
}

// MARK: DDC/CI

// probeDDC looks for DDC/CI on the monitor, in the background. A monitor
// that doesn't answer simply gets no DDC controls.
func (s *store) probeDDC() {
	if s.probingDDC {
		return
	}
	s.probingDDC = true
	if old := s.ddc; old != nil {
		s.ddc = nil
		go old.Close()
	}
	m := s.monitor
	go func() {
		d, err := m.OpenDDC()
		var r ddcReading
		if err == nil {
			r = readDDC(d, m.Name, true)
		}
		s.post(func() {
			s.probingDDC = false
			s.ddc = d
			s.applyDDC(r)
		})
	}()
}

// ddcReading is what the monitor reported over DDC/CI.
type ddcReading struct {
	values    map[byte][2]int // value and maximum of each code that answered
	inputs    []mitv.Source
	refreshed bool // values only: inputs and controls were not read again
}

// readDDC reads the values the panel shows; with all, also the inputs the
// capability string lists and which picture controls answer. It blocks.
func readDDC(d *display.DDC, model string, all bool) ddcReading {
	r := ddcReading{values: map[byte][2]int{}, refreshed: !all}
	codes := []byte{display.VCPBrightness, display.VCPVolume, display.VCPInput}
	if all {
		for _, c := range controlCatalog {
			codes = append(codes, c.code)
		}
		for _, id := range display.ParseCapabilities(d.Capabilities())[display.VCPInput] {
			r.inputs = append(r.inputs, mitv.Source{ID: id, Name: display.InputNameFor(model, id)})
		}
	}
	for _, code := range codes {
		if v, max, ok := d.Get(code); ok && max > 0 {
			r.values[code] = [2]int{v, max}
		}
	}
	return r
}

func (s *store) applyDDC(r ddcReading) {
	if s.ddc == nil {
		s.hasBrightness, s.ddcVolume, s.ddcInputs, s.controls = false, false, nil, nil
		return
	}
	if s.maxima == nil {
		s.maxima = map[byte]int{}
	}
	for code, v := range r.values {
		s.maxima[code] = v[1]
	}
	percent := func(code byte) (float64, bool) {
		v, ok := r.values[code]
		return float64(v[0]) * 100 / float64(v[1]), ok
	}
	if v, ok := percent(display.VCPBrightness); ok {
		s.brightness, s.hasBrightness = v, true
	}
	if v, ok := percent(display.VCPVolume); ok {
		s.ddcVolume = true
		if s.link == nil && !s.chasing {
			s.volume, s.hasVolume = v, true
		}
	}
	if r.refreshed {
		s.applyInput(r)
		for i := range s.controls {
			if v, ok := r.values[s.controls[i].code]; ok {
				s.controls[i].value = float64(v[0])
			}
		}
		return
	}
	s.ddcInputs = r.inputs
	s.applyInput(r)
	s.controls = nil
	for _, c := range controlCatalog {
		if v, ok := r.values[c.code]; ok {
			s.controls = append(s.controls, control{code: c.code, name: c.name, value: float64(v[0]), max: v[1]})
		}
	}
}

// applyInput highlights the input the monitor reports, when it is one of
// those it lists. Some report something else (issue #2: 8 where it lists
// 0x0F 0x10 0x11 0x12): the input chosen in the app stays highlighted then.
func (s *store) applyInput(r ddcReading) {
	v, ok := r.values[display.VCPInput]
	if !ok || s.link != nil {
		return
	}
	code := v[0] & 0xFF // some monitors put more in the high byte
	for _, src := range s.ddcInputs {
		if src.ID == code {
			s.input = code
		}
	}
}

// refreshDDC rereads the values the monitor's own buttons may have changed.
func (s *store) refreshDDC() {
	d := s.ddc
	if d == nil || s.probingDDC {
		return
	}
	model := s.monitor.Name
	go func() {
		r := readDDC(d, model, false)
		s.post(func() {
			if s.ddc == d {
				s.applyDDC(r)
			}
		})
	}()
}

// setBrightness sets the backlight over DDC/CI.
func (s *store) setBrightness(percent float64) {
	s.brightness = percent
	s.setDDC(display.VCPBrightness, percent)
}

// setDDC writes a percentage scaled to the code's range.
func (s *store) setDDC(code byte, percent float64) {
	if s.ddc == nil {
		return
	}
	max := s.maxima[code]
	if max == 0 {
		max = 100
	}
	s.ddc.Set(code, int(math.Round(percent/100*float64(max))))
}

func (s *store) setControl(i int) {
	if s.ddc != nil {
		s.ddc.Set(s.controls[i].code, int(math.Round(s.controls[i].value)))
	}
}

// MARK: Day and night

// autoCheck applies the day or night scene when the period changed since
// the last one applied, so manual changes stay until the next switch.
func (s *store) autoCheck() {
	if !s.prefs.AutoSwitch {
		return
	}
	period, _ := s.autoTimes().At(s.now())
	if string(period) == s.prefs.AutoPeriod {
		return
	}
	name := s.prefs.DayScene
	if period == schedule.Night {
		name = s.prefs.NightScene
	}
	for _, sc := range scenes {
		if sc.name == name {
			s.apply(sc)
		}
	}
	s.prefs.AutoPeriod, s.dirty = string(period), true
}

// autoChanged takes new settings: the current period's scene applies now.
func (s *store) autoChanged() {
	s.prefs.DayStart, s.prefs.NightStart = s.dayStart.Format("15:04"), s.nightStart.Format("15:04")
	s.prefs.AutoPeriod, s.dirty = "", true
	s.autoCheck()
}

func (s *store) autoTimes() schedule.Times {
	return schedule.Times{
		DayStart:   s.dayStart.Hour()*60 + s.dayStart.Minute(),
		NightStart: s.nightStart.Hour()*60 + s.nightStart.Minute(),
	}
}

// autoStatus says what is on and what comes next.
func (s *store) autoStatus() string {
	period, next := s.autoTimes().At(s.now())
	if period == schedule.Day {
		return fmt.Sprintf("现在是白天，%s 切换到「%s」", next.Format("15:04"), s.prefs.NightScene)
	}
	return fmt.Sprintf("现在是夜晚，%s 切换到「%s」", next.Format("15:04"), s.prefs.DayScene)
}

// parseClock reads "15:04" into a time of day, or the fallback minutes.
func parseClock(text string, fallback int) time.Time {
	t, err := time.Parse("15:04", text)
	if err != nil {
		t = time.Date(0, 1, 1, fallback/60, fallback%60, 0, 0, time.UTC)
	}
	return t
}

// MARK: Following the built-in display

// canFollow tells whether following the built-in display is possible: a
// MacBook's display to follow, and a backlight to set over DDC/CI.
func (s *store) canFollow() bool { return s.hasBuiltin && s.hasBrightness }

// followTick runs every second. A change of the built-in display's
// brightness sets a new target, which the backlight eases toward; between
// changes, a manual change of the backlight stays.
func (s *store) followTick(builtin func() (float64, bool)) {
	if !s.prefs.FollowBuiltin || !s.canFollow() {
		return
	}
	b, ok := builtin()
	if !ok {
		return
	}
	if s.lastBuiltin < 0 || math.Abs(b-s.lastBuiltin) >= 0.005 {
		s.lastBuiltin = b
		s.followTarget, s.easing = followTarget(b, s.prefs.FollowMin, s.prefs.FollowMax), true
	}
	if !s.easing {
		return
	}
	next := easeToward(s.brightness, s.followTarget)
	if math.Round(next) != math.Round(s.brightness) {
		s.setBrightness(next)
	} else {
		s.brightness = next
	}
	if next == s.followTarget {
		s.easing = false
	}
}

// followChanged restarts following from the built-in display's brightness.
func (s *store) followChanged() {
	s.lastBuiltin, s.dirty = -1, true
}

// followTarget maps the built-in display's brightness, 0...1, into the
// external backlight's range, in percent.
func followTarget(builtin, lo, hi float64) float64 {
	return lo + (hi-lo)*min(max(builtin, 0), 1)
}

// easeToward moves a third of the way to the target each second, at least
// one point, landing on it: a few seconds for any change, without jumps.
func easeToward(current, target float64) float64 {
	d := target - current
	if math.Abs(d) <= 1 {
		return target
	}
	step := d * 0.3
	if math.Abs(step) < 1 {
		step = math.Copysign(1, d)
	}
	return current + step
}

// MARK: Updates

// checkUpdate asks the releases for a newer version, in the background;
// manual checks also tell when there is none.
func (s *store) checkUpdate(manual bool) {
	if !s.canUpdate || s.installing {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		up, err := mygo.Updater.Check(ctx)
		s.post(func() {
			if err == nil {
				s.update = up
			}
			switch {
			case !manual:
			case err != nil:
				s.notice = "检查更新失败：网络不通或 GitHub 无法访问"
			case up == nil:
				s.notice = "已是最新版本 " + mygo.App.Version()
			}
		})
	}()
}

// installUpdate downloads the update, checks its signature, replaces the
// app and starts the new version.
func (s *store) installUpdate() {
	up := s.update
	if up == nil || s.installing {
		return
	}
	s.installing, s.progress, s.notice = true, 0, ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		shown := -1
		err := up.Install(ctx, func(done, total int64) {
			if total <= 0 {
				return
			}
			if p := int(done * 100 / total); p != shown {
				shown = p
				s.post(func() { s.progress = p })
			}
		})
		s.post(func() {
			if err != nil {
				s.installing, s.notice = false, "更新失败，请稍后再试或从 GitHub 下载"
				return
			}
			s.save()
			mygo.App.Relaunch()
		})
	}()
}

// MARK: Display modes

func (s *store) applyMode(m display.Mode) {
	if err := s.monitor.Apply(m); err != nil {
		s.notice = "切换失败，显示器拒绝了该模式"
	} else {
		s.notice = "已切换到 " + m.Title() + " · " + strconv.Itoa(m.RefreshRate) + " Hz"
	}
	s.refreshDisplay()
}

func (s *store) sleepDisplay() { _ = display.SleepNow() }

// MARK: Lifecycle

func (s *store) refreshDisplay() {
	_, s.hasBuiltin = display.BuiltinBrightness()
	s.monitor = display.Target()
	s.modes = s.monitor.Modes()
	s.current, s.hasCurrent = s.monitor.CurrentMode()
}

// displaysChanged follows plugging, mode changes and wake: the gamma table
// was reset, so capture it again and reapply.
func (s *store) displaysChanged() {
	previous := s.monitor
	s.refreshDisplay()
	display.ResetGammaBase()
	s.applyGamma()
	if s.monitor != previous {
		s.probeDDC()
	}
}

func (s *store) panelOpened() {
	s.usage = s.tracker.Recent(7)
	// Retry here too: the launch attempt may have raced the network
	// permission prompt.
	if s.link == nil {
		s.connect()
	} else {
		s.readVolume()
	}
	s.refreshDDC()
}

// detect looks for every control channel again.
func (s *store) detect() {
	s.connect()
	s.probeDDC()
}

// detectingAny reports a channel still being looked for.
func (s *store) detectingAny() bool { return s.detecting || s.probingDDC }

func (s *store) recordUsage() {
	s.autoCheck()
	s.tracker.Record(!s.monitor.Asleep(), time.Duration(display.IdleSeconds()*float64(time.Second)))
	s.usage = s.tracker.Recent(7)
	s.save()
}

func (s *store) toggleLaunchAtLogin() {
	if err := mygo.App.SetOpenAtLogin(!s.launchAtLogin); err != nil {
		s.notice = "登录项设置失败：" + err.Error()
	}
	s.launchAtLogin = mygo.App.OpenAtLogin()
}

func (s *store) save() {
	if s.dirty {
		s.prefs.save()
		s.dirty = false
	}
}
