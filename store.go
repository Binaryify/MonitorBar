package main

import (
	"errors"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
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
}

func newStore() *store {
	s := &store{
		prefs:         loadSettings(),
		tracker:       usage.Open(filepath.Join(dataDir(), "usage.json")),
		redraw:        func() {},
		launchAtLogin: mygo.App.OpenAtLogin(),
	}
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

func (s *store) apply(sc scene) {
	s.prefs.SoftBrightness, s.prefs.Warmth = sc.brightness, sc.warmth
	s.applyGamma()
}

func (s *store) isActive(sc scene) bool {
	return math.Abs(s.prefs.SoftBrightness-sc.brightness) < 1 && math.Abs(s.prefs.Warmth-sc.warmth) < 1
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

func (s *store) hasHardwareControl() bool { return s.link != nil }

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
		if _, err := link.DeviceName(); cached == "" || err != nil {
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
				s.inputs, s.hasVolume = nil, false
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
	s.monitor = display.Target()
	s.modes = s.monitor.Modes()
	s.current, s.hasCurrent = s.monitor.CurrentMode()
}

// displaysChanged follows plugging, mode changes and wake: the gamma table
// was reset, so capture it again and reapply.
func (s *store) displaysChanged() {
	s.refreshDisplay()
	display.ResetGammaBase()
	s.applyGamma()
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
}

func (s *store) recordUsage() {
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
