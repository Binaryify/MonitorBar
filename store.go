package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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

// screen is one connected display and the channels that reach it.
// The store reads and writes screens on the main thread only.
type screen struct {
	monitor    display.Monitor
	modes      []display.Mode
	current    display.Mode
	hasCurrent bool

	soft, warmth float64 // software dimming and warmth, percent

	link      *mitv.Link // this display's LAN remote, nil until found
	detecting bool
	inputs    []mitv.Source
	input     int // the input chosen in the app, 0 before any
	volume    float64
	hasVolume bool

	volumeTarget atomic.Int64
	deviceVolume int
	chasing      bool

	ddc           *display.DDC
	probingDDC    bool
	probeGen      int
	brightness    float64
	hasBrightness bool
	ddcInputs     []mitv.Source
	ddcVolume     bool
	controls      []control
	maxima        map[byte]int
}

func (sc *screen) refresh() {
	sc.modes = sc.monitor.Modes()
	sc.current, sc.hasCurrent = sc.monitor.CurrentMode()
}

// store holds what the panel shows and the actions that change it. Its
// fields are read and written on the main thread only: network work runs in
// goroutines and hands its results back through post.
type store struct {
	prefs    *settings
	tracker  *usage.Tracker
	redraw   func()                                                    // asks the open panel for a new frame
	setGamma func(m display.Monitor, brightness, warmth float64) error // replaced in tests

	screens  []*screen
	selected int
	cur      *screen // screens[selected]

	scanning bool // a LAN scan is in flight

	usage         []usage.Day
	launchAtLogin bool
	notice        string
	dirty         bool // prefs changed since the last save

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
	s.setGamma = func(m display.Monitor, b, w float64) error { return m.SetGamma(b, w) }
	s.reload()
	for _, sc := range s.screens {
		s.writeGamma(sc)
	}
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

// reload rebuilds the screen list from the displays currently attached,
// keeping the channels of a display that stayed. The first display inherits
// settings saved before there were per-display settings.
func (s *store) reload() {
	_, s.hasBuiltin = display.BuiltinBrightness()
	monitors := display.Monitors()
	if len(monitors) == 0 {
		monitors = []display.Monitor{{Name: "显示器", ID: "default"}}
	}
	old := map[string]*screen{}
	for _, sc := range s.screens {
		old[sc.monitor.ID] = sc
	}
	migrated := s.prefs.Monitors != nil
	var next []*screen
	for _, m := range monitors {
		if m.ID == "" {
			m.ID = "default"
		}
		if sc, ok := old[m.ID]; ok {
			sc.monitor = m
			sc.refresh()
			next = append(next, sc)
			delete(old, m.ID)
			continue
		}
		sc := &screen{monitor: m, soft: 100}
		if !migrated {
			if s.prefs.SoftBrightness > 0 {
				sc.soft = s.prefs.SoftBrightness
			}
			sc.warmth = s.prefs.Warmth
			if s.prefs.Monitors == nil {
				s.prefs.Monitors = map[string]monitorSettings{}
			}
			s.prefs.Monitors[m.ID] = monitorSettings{SoftBrightness: sc.soft, Warmth: sc.warmth, NetworkHost: s.prefs.NetworkHost}
			if s.prefs.Selected == "" {
				s.prefs.Selected = m.ID
			}
			migrated = true
			s.dirty = true
		} else if ms, ok := s.prefs.Monitors[m.ID]; ok {
			if ms.SoftBrightness > 0 {
				sc.soft = ms.SoftBrightness
			}
			sc.warmth = ms.Warmth
		}
		sc.refresh()
		next = append(next, sc)
	}
	for _, sc := range old {
		if sc.ddc != nil {
			go sc.ddc.Close()
		}
	}
	s.screens = next
	s.selected = 0
	for i, sc := range s.screens {
		if sc.monitor.ID == s.prefs.Selected {
			s.selected = i
		}
	}
	s.cur = s.screens[s.selected]
}

// selectScreen shows another display. Each keeps its own picture.
func (s *store) selectScreen(i int) {
	if i < 0 || i >= len(s.screens) || i == s.selected {
		return
	}
	s.selected = i
	s.cur = s.screens[i]
	s.prefs.Selected = s.cur.monitor.ID
	s.prefs.SoftBrightness, s.prefs.Warmth = s.cur.soft, s.cur.warmth
	s.dirty = true
	s.notice = ""
}

// screenLabel is the switcher's name. Two displays with the same name are
// told apart by the cable, then by position.
func (s *store) screenLabel(i int) string {
	sc := s.screens[i]
	name := sc.monitor.Name
	if name == "" {
		name = "显示器"
	}
	dup := 0
	for _, o := range s.screens {
		if o.monitor.Name == sc.monitor.Name {
			dup++
		}
	}
	switch {
	case dup > 1 && sc.monitor.Connection != "":
		return name + " " + sc.monitor.Connection
	case dup > 1:
		return fmt.Sprintf("%s %d", name, i+1)
	default:
		return name
	}
}

// remember writes one display's picture back into settings.
func (s *store) remember(sc *screen) {
	if s.prefs.Monitors == nil {
		s.prefs.Monitors = map[string]monitorSettings{}
	}
	prev := s.prefs.Monitors[sc.monitor.ID]
	host := prev.NetworkHost
	if sc.link != nil {
		host = sc.link.Host
	}
	s.prefs.Monitors[sc.monitor.ID] = monitorSettings{SoftBrightness: sc.soft, Warmth: sc.warmth, NetworkHost: host}
	if sc == s.cur {
		s.prefs.SoftBrightness, s.prefs.Warmth = sc.soft, sc.warmth
		s.prefs.Selected = sc.monitor.ID
		if host != "" {
			s.prefs.NetworkHost = host
		}
	}
}

// MARK: Picture (software, through the gamma table)

func (s *store) setSoftBrightness(percent float64) {
	s.cur.soft = percent
	s.writeGamma(s.cur)
}

func (s *store) setWarmth(percent float64) {
	s.cur.warmth = percent
	s.writeGamma(s.cur)
}

// apply sets a scene on the display the panel is showing.
func (s *store) apply(sc scene) { s.applyScene(s.cur, sc) }

// applyScene sets a scene's brightness, on the backlight when DDC/CI
// reaches it, and its warmth.
func (s *store) applyScene(scr *screen, sc scene) {
	if scr.hasBrightness {
		scr.brightness = sc.brightness
		s.writeDDC(scr, display.VCPBrightness, sc.brightness)
	} else {
		scr.soft = sc.brightness
	}
	scr.warmth = sc.warmth
	s.writeGamma(scr)
}

func (s *store) isActive(sc scene) bool {
	return math.Abs(s.effectiveBrightness()-sc.brightness) < 1 && math.Abs(s.cur.warmth-sc.warmth) < 1
}

// effectiveBrightness is the backlight's when DDC/CI reaches it, else the
// software dimming's.
func (s *store) effectiveBrightness() float64 {
	if s.cur.hasBrightness {
		return s.cur.brightness
	}
	return s.cur.soft
}

func (s *store) resetSoftware() {
	s.cur.soft, s.cur.warmth = 100, 0
	s.writeGamma(s.cur)
}

const (
	gammaLimited = "Windows 限制了软件调光的幅度，已调到它允许的最暗。"
	gammaFailed  = "调光失败：系统拒绝了这个设置。"
)

func (s *store) writeGamma(sc *screen) {
	err := s.setGamma(sc.monitor, sc.soft/100, sc.warmth/100)
	s.remember(sc)
	s.dirty = true
	if sc != s.cur {
		return
	}
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
	if s.cur.link != nil {
		return s.cur.inputs
	}
	return s.cur.ddcInputs
}

// inputChannel names how inputs are switched, for the inputs card.
func (s *store) inputChannel() string {
	if s.cur.link != nil {
		return "局域网"
	}
	return "DDC/CI"
}

// lanHit is one HyperOS device a scan found, with what the panel shows.
type lanHit struct {
	link      *mitv.Link
	sources   []mitv.Source
	volume    int
	volumeErr error
}

// connect finds the displays on the LAN. A display with a remembered
// address is tried first; the rest of a scan fills displays that have none.
func (s *store) connect() {
	if s.scanning {
		return
	}
	s.scanning = true
	saved := map[string]string{}
	for _, sc := range s.screens {
		if sc.link == nil {
			sc.detecting = true
			saved[sc.monitor.ID] = s.prefs.hostFor(sc.monitor.ID)
		}
	}
	go func() {
		live := map[string]lanHit{}
		try := func(host string) {
			if host == "" {
				return
			}
			if _, ok := live[host]; ok {
				return
			}
			link := mitv.New(host)
			// The host found last time must still be a monitor: addresses move.
			if name, err := link.DeviceName(); err != nil || !mitv.IsMonitor(name) {
				return
			}
			sources, err := link.Sources()
			if err != nil || len(sources) == 0 {
				return
			}
			volume, volumeErr := link.Volume()
			live[host] = lanHit{link: link, sources: sources, volume: volume, volumeErr: volumeErr}
		}
		needScan := false
		for _, host := range saved {
			try(host)
			if host == "" || live[host].link == nil {
				needScan = true
			}
		}
		if needScan {
			for _, host := range mitv.DiscoverAll() {
				try(host)
			}
		}
		s.post(func() {
			s.scanning = false
			s.assign(live)
		})
	}()
}

// assign gives each display its remembered device, then any device still
// left over, one each.
func (s *store) assign(live map[string]lanHit) {
	used := map[string]bool{}
	for _, sc := range s.screens {
		if sc.link != nil {
			used[sc.link.Host] = true
			sc.detecting = false
			continue
		}
		host := s.prefs.hostFor(sc.monitor.ID)
		hit, ok := live[host]
		if !ok || used[host] {
			continue
		}
		s.attach(sc, hit)
		used[host] = true
	}
	var spare []lanHit
	for host, hit := range live {
		if !used[host] {
			spare = append(spare, hit)
		}
	}
	slices.SortFunc(spare, func(a, b lanHit) int { return strings.Compare(a.link.Host, b.link.Host) })
	for _, sc := range s.screens {
		if sc.link != nil || len(spare) == 0 {
			continue
		}
		s.attach(sc, spare[0])
		spare = spare[1:]
	}
	for _, sc := range s.screens {
		sc.detecting = false
		if sc.link == nil {
			sc.inputs = nil
			sc.hasVolume = sc.ddcVolume
		}
	}
}

func (s *store) attach(sc *screen, hit lanHit) {
	slices.SortFunc(hit.sources, func(a, b mitv.Source) int { return a.ID - b.ID })
	sc.link = hit.link
	sc.inputs = hit.sources
	sc.detecting = false
	if hit.volumeErr == nil {
		sc.deviceVolume, sc.volume, sc.hasVolume = hit.volume, float64(hit.volume), true
	}
	s.remember(sc)
}

func (s *store) selectInput(src mitv.Source) {
	sc := s.cur
	sc.input = src.ID
	link := sc.link
	if link == nil {
		if sc.ddc != nil {
			sc.ddc.Set(display.VCPInput, src.ID)
		}
		return
	}
	go func() {
		if err := link.Select(src.ID); err != nil {
			s.post(func() {
				if s.cur == sc {
					s.notice = "切换到 " + src.Name + " 失败：显示器没有响应"
				}
			})
		}
	}()
}

func (s *store) setVolume(percent float64) {
	sc := s.cur
	sc.volume = percent
	if sc.link == nil {
		s.writeDDC(sc, display.VCPVolume, percent)
		return
	}
	sc.volumeTarget.Store(int64(math.Round(percent)))
	s.chaseVolume(sc)
}

// chaseVolume steps toward the latest target: the API only has ±1 keys.
func (s *store) chaseVolume(sc *screen) {
	if sc.link == nil || sc.chasing {
		return
	}
	sc.chasing = true
	link, actual := sc.link, sc.deviceVolume
	go func() {
		failed := false
		for target := int(sc.volumeTarget.Load()); target != actual; target = int(sc.volumeTarget.Load()) {
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
			if !slices.Contains(s.screens, sc) {
				return
			}
			sc.chasing = false
			sc.deviceVolume = actual
			switch {
			case failed && s.cur == sc:
				s.notice = "调节音量失败：显示器没有响应"
			case int(sc.volumeTarget.Load()) != actual:
				s.chaseVolume(sc) // the slider moved while the last step was sent
			default:
				s.readVolume(sc)
			}
		})
	}()
}

func (s *store) readVolume(sc *screen) {
	link := sc.link
	if link == nil {
		return
	}
	go func() {
		v, err := link.Volume()
		if err != nil {
			return
		}
		s.post(func() {
			if !slices.Contains(s.screens, sc) {
				return
			}
			sc.deviceVolume, sc.hasVolume = v, true
			if !sc.chasing {
				sc.volume = float64(v)
			}
		})
	}()
}

// MARK: DDC/CI

// probeScreen looks for DDC/CI on one display, in the background. A monitor
// that doesn't answer simply gets no DDC controls.
func (s *store) probeScreen(sc *screen) {
	sc.probeGen++
	gen := sc.probeGen
	sc.probingDDC = true
	if old := sc.ddc; old != nil {
		sc.ddc = nil
		go old.Close()
	}
	m := sc.monitor
	go func() {
		d, err := m.OpenDDC()
		var r ddcReading
		if err == nil {
			r = readDDC(d, m.Name, true)
		}
		s.post(func() {
			if sc.probeGen != gen || !slices.Contains(s.screens, sc) {
				if d != nil {
					go d.Close()
				}
				return
			}
			sc.probingDDC = false
			sc.ddc = d
			s.applyDDC(sc, r)
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

func (s *store) applyDDC(sc *screen, r ddcReading) {
	if sc.ddc == nil {
		sc.hasBrightness, sc.ddcVolume, sc.ddcInputs, sc.controls = false, false, nil, nil
		return
	}
	if sc.maxima == nil {
		sc.maxima = map[byte]int{}
	}
	for code, v := range r.values {
		sc.maxima[code] = v[1]
	}
	percent := func(code byte) (float64, bool) {
		v, ok := r.values[code]
		return float64(v[0]) * 100 / float64(v[1]), ok
	}
	if v, ok := percent(display.VCPBrightness); ok {
		sc.brightness, sc.hasBrightness = v, true
	}
	if v, ok := percent(display.VCPVolume); ok {
		sc.ddcVolume = true
		if sc.link == nil && !sc.chasing {
			sc.volume, sc.hasVolume = v, true
		}
	}
	if r.refreshed {
		s.applyInput(sc, r)
		for i := range sc.controls {
			if v, ok := r.values[sc.controls[i].code]; ok {
				sc.controls[i].value = float64(v[0])
			}
		}
		return
	}
	sc.ddcInputs = r.inputs
	s.applyInput(sc, r)
	sc.controls = nil
	for _, c := range controlCatalog {
		if v, ok := r.values[c.code]; ok {
			sc.controls = append(sc.controls, control{code: c.code, name: c.name, value: float64(v[0]), max: v[1]})
		}
	}
}

// applyInput highlights the input the monitor reports, when it is one of
// those it lists. Some report something else (issue #2: 8 where it lists
// 0x0F 0x10 0x11 0x12): the input chosen in the app stays highlighted then.
func (s *store) applyInput(sc *screen, r ddcReading) {
	v, ok := r.values[display.VCPInput]
	if !ok || sc.link != nil {
		return
	}
	code := v[0] & 0xFF // some monitors put more in the high byte
	for _, src := range sc.ddcInputs {
		if src.ID == code {
			sc.input = code
		}
	}
}

// refreshDDC rereads the values the monitor's own buttons may have changed.
func (s *store) refreshDDC(sc *screen) {
	d := sc.ddc
	if d == nil || sc.probingDDC {
		return
	}
	model := sc.monitor.Name
	go func() {
		r := readDDC(d, model, false)
		s.post(func() {
			if sc.ddc == d {
				s.applyDDC(sc, r)
			}
		})
	}()
}

// setBrightness sets the backlight over DDC/CI.
func (s *store) setBrightness(percent float64) {
	s.cur.brightness = percent
	s.writeDDC(s.cur, display.VCPBrightness, percent)
}

// writeDDC writes a percentage scaled to the code's range.
func (s *store) writeDDC(sc *screen, code byte, percent float64) {
	if sc.ddc == nil {
		return
	}
	max := sc.maxima[code]
	if max == 0 {
		max = 100
	}
	sc.ddc.Set(code, int(math.Round(percent/100*float64(max))))
}

func (s *store) setControl(i int) {
	sc := s.cur
	if sc.ddc != nil && i >= 0 && i < len(sc.controls) {
		sc.ddc.Set(sc.controls[i].code, int(math.Round(sc.controls[i].value)))
	}
}

// MARK: Day and night

// autoCheck applies the day or night scene when the period changed since
// the last one applied, so manual changes stay until the next switch.
// The scene is applied to every display.
func (s *store) autoCheck() {
	if !s.prefs.AutoSwitch || len(s.screens) == 0 {
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
			for _, scr := range s.screens {
				s.applyScene(scr, sc)
			}
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
func (s *store) canFollow() bool {
	return s.hasBuiltin && s.cur != nil && s.cur.hasBrightness
}

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
	next := easeToward(s.cur.brightness, s.followTarget)
	if math.Round(next) != math.Round(s.cur.brightness) {
		s.setBrightness(next)
	} else {
		s.cur.brightness = next
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
	sc := s.cur
	if err := sc.monitor.Apply(m); err != nil {
		s.notice = "切换失败，显示器拒绝了该模式"
	} else {
		s.notice = "已切换到 " + m.Title() + " · " + strconv.Itoa(m.RefreshRate) + " Hz"
	}
	sc.refresh()
}

func (s *store) sleepDisplay() { _ = display.SleepNow() }

// MARK: Lifecycle

// displaysChanged follows plugging, mode changes and wake: the gamma tables
// were reset, so capture them again and reapply each display's picture.
func (s *store) displaysChanged() {
	prev := map[string]display.Monitor{}
	for _, sc := range s.screens {
		prev[sc.monitor.ID] = sc.monitor
	}
	s.reload()
	display.ResetGammaBase()
	for _, sc := range s.screens {
		s.writeGamma(sc)
		old, ok := prev[sc.monitor.ID]
		if !ok || !old.SameOutput(sc.monitor) {
			s.probeScreen(sc)
		}
	}
}

func (s *store) panelOpened() {
	s.usage = s.tracker.Recent(7)
	// Retry here too: the launch attempt may have raced the network
	// permission prompt.
	missing := false
	for _, sc := range s.screens {
		if sc.link == nil {
			missing = true
		} else {
			s.readVolume(sc)
		}
	}
	if missing {
		s.connect()
	}
	for _, sc := range s.screens {
		s.refreshDDC(sc)
	}
}

// detect looks for every control channel again, on every display.
func (s *store) detect() {
	for _, sc := range s.screens {
		sc.link = nil
		s.probeScreen(sc)
	}
	s.connect()
}

// detectingAny reports a channel of the display on screen still being looked for.
func (s *store) detectingAny() bool {
	if s.cur == nil {
		return s.scanning
	}
	return s.cur.detecting || s.cur.probingDDC || (s.scanning && s.cur.link == nil)
}

func (s *store) recordUsage() {
	s.autoCheck()
	awake := false
	for _, sc := range s.screens {
		if !sc.monitor.Asleep() {
			awake = true
			break
		}
	}
	s.tracker.Record(awake, time.Duration(display.IdleSeconds()*float64(time.Second)))
	s.usage = s.tracker.Recent(7)
	s.save()
}

func (s *store) toggleLaunchAtLogin() {
	if err := mygo.App.SetOpenAtLogin(!s.launchAtLogin); err != nil {
		s.notice = "登录项设置失败：" + err.Error()
	}
	s.launchAtLogin = mygo.App.OpenAtLogin()
}

func (s *store) close() {
	for _, sc := range s.screens {
		if sc.ddc != nil {
			sc.ddc.Close()
			sc.ddc = nil
		}
	}
}

func (s *store) save() {
	if s.dirty {
		s.prefs.save()
		s.dirty = false
	}
}
