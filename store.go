package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/keystate"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/schedule"
	"github.com/binaryify/MonitorBar/internal/usage"
	"github.com/binaryify/MonitorBar/locales"
)

// scene sets brightness and warmth at once. Its id is kept in the
// settings, for switching by the time.
type scene struct {
	id                 string
	label              func() string
	icon               string
	brightness, warmth float64     // a built-in scene's brightness is the backlight's when DDC/CI reaches it
	saved              *savedScene // the user's, nil for a built-in scene
}

var scenes = []scene{
	{id: "day", label: locales.SceneDay, icon: "sun", brightness: 80},
	{id: "reading", label: locales.SceneReading, icon: "book", brightness: 55, warmth: 25},
	{id: "movie", label: locales.SceneMovie, icon: "film", brightness: 100, warmth: 5},
	{id: "night", label: locales.SceneNight, icon: "moon", brightness: 30, warmth: 55},
}

// levels is the backlight the scene sets, -1 to leave it, and the software
// dimming, on a monitor whose backlight DDC/CI reaches or not.
func (sc scene) levels(backlight bool) (float64, float64) {
	switch {
	case sc.saved != nil && sc.saved.Backlight != nil:
		return *sc.saved.Backlight, sc.saved.Soft
	case sc.saved != nil:
		return -1, sc.saved.Soft
	case backlight:
		return sc.brightness, 100 // a built-in scene's brightness is all backlight
	}
	return -1, sc.brightness
}

// maxSavedScenes keeps the scenes in three rows of chips.
const maxSavedScenes = 8

// allScenes are the built-in scenes, then those the user saved.
func (s *store) allScenes() []scene {
	all := slices.Clone(scenes)
	for i := range s.prefs.SavedScenes {
		saved := &s.prefs.SavedScenes[i]
		name := saved.Name
		all = append(all, scene{id: saved.ID, label: func() string { return cmp.Or(name, locales.SceneUntitled()) },
			icon: "star", warmth: saved.Warmth, saved: saved})
	}
	return all
}

// sceneByID finds a scene, the first for an id no scene has.
func (s *store) sceneByID(id string) scene {
	all := s.allScenes()
	for _, sc := range all {
		if sc.id == id {
			return sc
		}
	}
	return all[0]
}

// saveScene keeps the current picture as a new scene.
func (s *store) saveScene() {
	if len(s.prefs.SavedScenes) >= maxSavedScenes {
		return
	}
	n := 1
	for slices.ContainsFunc(s.prefs.SavedScenes, func(sc savedScene) bool { return sc.ID == fmt.Sprintf("saved-%d", n) }) {
		n++
	}
	sc := savedScene{ID: fmt.Sprintf("saved-%d", n), Name: locales.SceneCustomName(n), Soft: s.soft, Warmth: s.warmth}
	if s.hasBrightness {
		sc.Backlight = new(s.brightness)
	}
	s.prefs.SavedScenes, s.dirty = append(s.prefs.SavedScenes, sc), true
}

// deleteScene removes a saved scene; switching by the time goes back to the
// built-in scene where it had chosen it.
func (s *store) deleteScene(id string) {
	s.prefs.SavedScenes = slices.DeleteFunc(s.prefs.SavedScenes, func(sc savedScene) bool { return sc.ID == id })
	if s.prefs.DayScene == id {
		s.prefs.DayScene = "day"
	}
	if s.prefs.NightScene == id {
		s.prefs.NightScene = "night"
	}
	s.dirty = true
}

// store holds what the panel shows and the actions that change it. Its
// fields are read and written on the main thread only: network work runs in
// goroutines and hands its results back through post.
type store struct {
	// The display the panel shows and its fields are of: on and every
	// make another one it for a while, so actions work on each the same.
	*output
	outputs []*output // every display managed, the main one first

	prefs    *settings
	tracker  *usage.Tracker
	redraw   func()                                 // asks the open panel for a new frame
	setGamma func(brightness, warmth float64) error // the display's, replaced in tests

	detecting bool
	lan       []lanMonitor // the HyperOS monitors found on the LAN

	usage         []usage.Day
	launchAtLogin bool

	// Shortcuts: those bound, by id, and the one the panel records keys for.
	bindShortcuts bool // false in tests, which have no app to bind them
	registered    map[string]string
	recording     string
	held          string                 // the shortcut whose keys are held, repeating it
	keyHeld       func(keys string) bool // whether keys are still down, fixed in tests
	// onAdjust shows the HUD for a level a key just set.
	onAdjust func(hudKind)
	// The media keys: taken where the user asked and macOS allows.
	audioRoute      func() display.AudioRoute // whether the sound goes to the display, fixed in tests
	mediaKeysDenied bool                      // asked for, without the Accessibility permission

	notice string
	dirty  bool // prefs changed since the last save

	// The day and night start times as the time inputs edit them.
	dayStart, nightStart time.Time
	now                  func() time.Time // the clock, fixed in tests

	// Updates, for builds that can update themselves.
	canUpdate  bool
	update     *mygo.Update // a newer version, nil when up to date
	installing bool
	progress   int // percent downloaded

	hasBuiltin bool // a MacBook's own display, to follow
}

// output is a display and what reaches it.
type output struct {
	monitor     display.Monitor
	modes       []display.Mode
	current     display.Mode
	hasCurrent  bool
	switchingTo string // the mode the LAN remote is switching the monitor to

	// The software picture, through the gamma table.
	soft   float64 // gamma %, 20...100
	warmth float64 // 0...100

	link      *mitv.Link // the display's LAN remote, nil until found
	inputs    []mitv.Source
	input     int // the input chosen in the app, 0 before any
	volume    float64
	hasVolume bool
	unmuted   float64 // the volume to go back to from mute, 0 when not muted

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

	// Following the built-in display: its brightness last seen (-1 before
	// any), and the backlight being eased toward.
	lastBuiltin  float64
	followTarget float64
	easing       bool
}

// on runs fn with o as the display the store's fields are of.
func (s *store) on(o *output, fn func()) {
	shown := s.output
	s.output = o
	defer func() { s.output = shown }()
	fn()
}

// every runs fn on each display.
func (s *store) every(fn func()) {
	for _, o := range s.outputs {
		s.on(o, fn)
	}
}

// each runs fn on the display shown, or on every one while they are kept
// in sync (#14).
func (s *store) each(fn func()) {
	if s.prefs.SyncDisplays {
		s.every(fn)
	} else {
		fn()
	}
}

// control is a picture setting the monitor offers over DDC/CI.
type control struct {
	code  byte
	label func() string
	value float64
	max   int
}

// controlCatalog lists the settings shown when the monitor answers them.
var controlCatalog = []struct {
	code  byte
	label func() string
}{
	{display.VCPContrast, locales.ControlContrast}, {display.VCPSharpness, locales.ControlSharpness},
	{display.VCPRedGain, locales.ControlRedGain}, {display.VCPGreenGain, locales.ControlGreenGain},
	{display.VCPBlueGain, locales.ControlBlueGain},
}

func newStore() *store {
	s := &store{
		prefs:         loadSettings(),
		tracker:       usage.Open(filepath.Join(dataDir(), "usage.json")),
		redraw:        func() {},
		now:           time.Now,
		launchAtLogin: mygo.App.OpenAtLogin(),
		canUpdate:     mygo.Updater.Enabled(),
		registered:    map[string]string{},
		audioRoute:    display.CurrentAudioRoute,
		keyHeld:       keystate.Held,
		onAdjust:      func(hudKind) {},
	}
	s.dayStart, s.nightStart = parseClock(s.prefs.DayStart, 7*60), parseClock(s.prefs.NightStart, 19*60)
	s.refreshDisplay()
	s.setGamma = func(b, w float64) error { return s.monitor.SetGamma(b, w) }
	s.every(s.applyGamma)
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
	s.soft = percent
	s.applyGamma()
}

func (s *store) setWarmth(percent float64) {
	s.warmth = percent
	s.applyGamma()
}

// apply sets a scene's brightness, on the backlight when DDC/CI reaches it,
// and its warmth.
func (s *store) apply(sc scene) {
	backlight, soft := sc.levels(s.hasBrightness)
	if s.hasBrightness && backlight >= 0 {
		s.setBrightness(backlight)
	}
	s.soft, s.warmth = soft, sc.warmth
	s.applyGamma()
}

func (s *store) isActive(sc scene) bool {
	backlight, soft := sc.levels(s.hasBrightness)
	return math.Abs(s.soft-soft) < 1 && math.Abs(s.warmth-sc.warmth) < 1 &&
		(!s.hasBrightness || backlight < 0 || math.Abs(s.brightness-backlight) < 1)
}

// Combined brightness, as BetterDisplay and MonitorControl have it (#9):
// one scale over the backlight and software dimming. Above the switchover
// the backlight goes from its minimum to its maximum; below it, the
// backlight stays at its minimum and the gamma table dims, down to softMin.
const (
	switchover = 50.0
	softMin    = 20.0 // the darkest software dimming, %
)

// combine places a backlight and a software dimming, in percent, on the
// combined scale.
func combine(hardware, soft float64) float64 {
	if hardware > 0 || soft >= 100 {
		return switchover + hardware*(100-switchover)/100
	}
	return (soft - softMin) / (100 - softMin) * switchover
}

// split is the backlight and the software dimming of a combined brightness.
func split(combined float64) (hardware, soft float64) {
	if combined >= switchover {
		return (combined - switchover) / (100 - switchover) * 100, 100
	}
	return 0, softMin + max(combined, 0)/switchover*(100-softMin)
}

// setLevel sets the brightness the overview shows: on the combined scale
// where DDC/CI reaches the backlight, else the software dimming. Displays
// kept in sync take the same level, whichever scale each has.
func (s *store) setLevel(level float64) {
	if s.hasBrightness {
		s.setCombined(level)
		return
	}
	s.soft = min(max(level, softMin), 100)
	s.applyGamma()
}

func (s *store) combinedBrightness() float64 { return combine(s.brightness, s.soft) }

// setCombined sets the backlight and the software dimming of a combined
// brightness, writing only what changed: DDC/CI is slow.
func (s *store) setCombined(combined float64) {
	hardware, soft := split(combined)
	if hardware != s.brightness {
		s.setBrightness(hardware)
	}
	if soft != s.soft {
		s.soft = soft
		s.applyGamma()
	}
}

// stepBrightness moves the brightness by delta points, on the combined
// scale when DDC/CI reaches the backlight.
func (s *store) stepBrightness(delta float64) {
	if s.hasBrightness {
		s.setCombined(min(max(s.combinedBrightness()+delta, 0), 100))
		return
	}
	s.soft = min(max(s.soft+delta, softMin), 100)
	s.applyGamma()
}

// effectiveBrightness is the backlight's when DDC/CI reaches it, else the
// software dimming's.
func (s *store) effectiveBrightness() float64 {
	if s.hasBrightness {
		return s.brightness
	}
	return s.soft
}

func (s *store) resetSoftware() {
	s.soft, s.warmth = 100, 0
	s.applyGamma()
}

func (s *store) applyGamma() {
	s.dirty = true
	err := s.setGamma(s.soft/100, s.warmth/100)
	switch {
	case errors.Is(err, display.ErrGammaLimited):
		s.notice = locales.NoticeGammaLimited()
	case err != nil:
		s.notice = locales.NoticeGammaFailed()
	case s.notice == locales.NoticeGammaLimited() || s.notice == locales.NoticeGammaFailed():
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
		return locales.InputsOverLan()
	}
	return "DDC/CI"
}

// lanMonitor is a HyperOS monitor found on the LAN, as it was then.
type lanMonitor struct {
	link      *mitv.Link
	name      string
	sources   []mitv.Source
	volume    int
	hasVolume bool
}

// connect finds the monitors on the LAN: the hosts paired last time, else
// a scan, then pairs them with the displays.
func (s *store) connect() {
	if s.detecting {
		return
	}
	s.detecting = true
	var saved []string
	for _, o := range s.outputs {
		if host := s.savedHost(o); host != "" && host != noLink && !slices.Contains(saved, host) {
			saved = append(saved, host)
		}
	}
	displays := len(s.outputs)
	go func() {
		// A host paired last time must still be a monitor: addresses move.
		hosts := slices.DeleteFunc(saved, func(h string) bool { return !mitv.New(h).IsMonitor() })
		if len(hosts) < displays {
			if found, err := mitv.Discover(); err == nil {
				hosts = found
			}
		}
		var found []lanMonitor
		for _, host := range hosts {
			link := mitv.New(host)
			sources, err := link.Sources()
			if err != nil || len(sources) == 0 {
				continue
			}
			slices.SortFunc(sources, func(a, b mitv.Source) int { return a.ID - b.ID })
			name, _ := link.DeviceName()
			volume, volumeErr := link.Volume()
			found = append(found, lanMonitor{link: link, name: name, sources: sources, volume: volume, hasVolume: volumeErr == nil})
		}
		s.post(func() {
			s.detecting = false
			s.lan = found
			s.pairLinks()
		})
	}()
}

// savedHost is the LAN remote paired with a display last time: the one
// MonitorBar used before it managed several displays, for the first.
func (s *store) savedHost(o *output) string {
	if d := s.prefs.Displays[o.monitor.Key]; d != nil && d.NetworkHost != "" {
		return d.NetworkHost
	}
	if len(s.outputs) > 0 && o == s.outputs[0] {
		return s.prefs.NetworkHost
	}
	return ""
}

// pairLinks pairs the monitors found on the LAN with the displays: as last
// time, then the others in turn, Xiaomi's displays first. Nothing tells two
// alike apart, so the user may pair them anew (choseLink).
func (s *store) pairLinks() {
	byHost := map[string]*lanMonitor{}
	for i := range s.lan {
		byHost[s.lan[i].link.Host] = &s.lan[i]
	}
	var unpaired []*output
	for _, o := range s.outputs {
		host := s.savedHost(o)
		if lm := byHost[host]; lm != nil {
			s.on(o, func() { s.pair(lm) })
			delete(byHost, host)
			continue
		}
		s.on(o, func() { s.pair(nil) })
		if host != noLink {
			unpaired = append(unpaired, o)
		}
	}
	slices.SortStableFunc(unpaired, func(a, b *output) int { return cmp.Compare(b2i(!xiaomi(a.monitor.Name)), b2i(!xiaomi(b.monitor.Name))) })
	for _, o := range unpaired {
		for i := range s.lan {
			if lm := &s.lan[i]; byHost[lm.link.Host] != nil {
				s.on(o, func() { s.pair(lm) })
				delete(byHost, lm.link.Host)
				break
			}
		}
	}
}

// xiaomi tells a display by Xiaomi from its name, the likeliest to have the
// LAN remote.
func xiaomi(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "xiaomi") || strings.Contains(name, "redmi") || strings.HasPrefix(name, "mi ")
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// pair makes lm the LAN remote of the display, nil for none.
func (s *store) pair(lm *lanMonitor) {
	if lm == nil {
		s.link, s.inputs, s.hasVolume = nil, nil, s.ddcVolume // DDC/CI may still have volume
		return
	}
	s.link, s.inputs = lm.link, lm.sources
	if lm.hasVolume {
		s.deviceVolume, s.volume, s.hasVolume = lm.volume, float64(lm.volume), true
	}
	s.prefs.display(s.monitor.Key).NetworkHost, s.dirty = lm.link.Host, true
	if len(s.outputs) > 0 && s.output == s.outputs[0] {
		s.prefs.NetworkHost = lm.link.Host
	}
}

// chooseLink pairs the display shown with the LAN monitor of host, "" for
// none. The display that had it takes the one this display had.
func (s *store) chooseLink(host string) {
	var mine string
	if s.link != nil {
		mine = s.link.Host
	}
	if host == mine {
		return
	}
	shown := s.output
	for _, o := range s.outputs {
		if o != shown && o.link != nil && o.link.Host == host {
			s.on(o, func() {
				s.pair(s.lanByHost(mine))
				if mine == "" {
					s.prefs.display(s.monitor.Key).NetworkHost = noLink
				}
			})
		}
	}
	s.pair(s.lanByHost(host))
	if host == "" {
		s.prefs.display(s.monitor.Key).NetworkHost, s.dirty = noLink, true
	}
	s.readVolume()
}

func (s *store) lanByHost(host string) *lanMonitor {
	for i := range s.lan {
		if s.lan[i].link.Host == host {
			return &s.lan[i]
		}
	}
	return nil
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
			s.post(func() { s.notice = locales.NoticeInputFailed(src.Name) })
		}
	}()
}

// stepVolume moves the volume by delta points, when it can be set.
func (s *store) stepVolume(delta float64) {
	if s.hasVolume {
		s.setVolume(min(max(s.volume+delta, 0), 100))
	}
}

// toggleMute takes the volume to 0, or back to what it was: neither the LAN
// remote nor every DDC/CI display has a mute.
func (s *store) toggleMute() {
	if !s.hasVolume {
		return
	}
	mute := s.volume > 0 // as the display shown is, on all in sync
	s.each(func() { s.setMuted(mute) })
}

func (s *store) setMuted(mute bool) {
	switch {
	case !s.hasVolume:
	case mute && s.volume > 0:
		s.unmuted = s.volume
		s.setVolume(0)
	case !mute && s.volume == 0:
		s.setVolume(cmp.Or(s.unmuted, defaultUnmuted))
	}
}

const defaultUnmuted = 20 // the volume unmuting goes to, when it was muted elsewhere

func (s *store) setVolume(percent float64) {
	if percent > 0 {
		s.unmuted = 0
	}
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
	o, link, actual := s.output, s.link, s.deviceVolume
	go func() {
		failed := false
		for target := int(o.volumeTarget.Load()); target != actual; target = int(o.volumeTarget.Load()) {
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
			s.on(o, func() {
				s.chasing = false
				s.deviceVolume = actual
				switch {
				case failed:
					s.notice = locales.NoticeVolumeFailed()
				case int(s.volumeTarget.Load()) != actual:
					s.chaseVolume() // the slider moved while the last step was sent
				default:
					s.readVolume()
				}
			})
		})
	}()
}

func (s *store) readVolume() {
	o, link := s.output, s.link
	if link == nil {
		return
	}
	go func() {
		v, err := link.Volume()
		if err != nil {
			return
		}
		s.post(func() {
			if o.link != link {
				return // paired anew meanwhile
			}
			o.deviceVolume, o.hasVolume = v, true
			if !o.chasing {
				o.volume = float64(v)
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
	o, m := s.output, s.monitor
	go func() {
		d, err := m.OpenDDC()
		var r ddcReading
		if err == nil {
			r = readDDC(d, m.Name, true)
		}
		s.post(func() {
			if !slices.Contains(s.outputs, o) { // unplugged meanwhile
				if d != nil {
					go d.Close()
				}
				return
			}
			s.on(o, func() {
				s.probingDDC = false
				s.ddc = d
				s.applyDDC(r)
			})
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
			s.controls = append(s.controls, control{code: c.code, label: c.label, value: float64(v[0]), max: v[1]})
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
	o, model := s.output, s.monitor.Name
	go func() {
		r := readDDC(d, model, false)
		s.post(func() {
			if o.ddc == d {
				s.on(o, func() { s.applyDDC(r) })
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
	id := s.prefs.DayScene
	if period == schedule.Night {
		id = s.prefs.NightScene
	}
	sc := s.sceneByID(id)
	s.every(func() { s.apply(sc) }) // every display, kept in sync or not
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
		return locales.AutoNowDay(next, s.sceneByID(s.prefs.NightScene).label())
	}
	return locales.AutoNowNight(next, s.sceneByID(s.prefs.DayScene).label())
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
	if s.soft < 100 { // following sets the backlight alone, with no software dimming
		s.soft = 100
		s.applyGamma()
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
	s.every(func() { s.lastBuiltin = -1 })
	s.dirty = true
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
				s.notice = locales.NoticeUpdateCheckFailed()
			case up == nil:
				s.notice = locales.NoticeUpToDate(mygo.App.Version())
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
				s.installing, s.notice = false, locales.NoticeUpdateFailed()
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
		s.notice = locales.NoticeModeRejected()
	} else {
		s.notice = locales.NoticeModeSwitched(m.Title(), m.RefreshRate)
	}
	s.refreshDisplay()
}

// competitive reports the monitor in its 1080p 320Hz mode: only then does
// the system offer rates above the 160Hz of 4K.
func (s *store) competitive() bool {
	return slices.ContainsFunc(s.modes, func(m display.Mode) bool { return m.RefreshRate > 160 })
}

// setCompetitive switches the monitor between 4K 160Hz and 1080p 320Hz
// through the LAN remote. The system follows once the monitor offers its
// new EDID, some seconds later.
func (s *store) setCompetitive(on bool) {
	link := s.link
	if link == nil || s.switchingTo != "" || on == s.competitive() {
		return
	}
	s.switchingTo = "4K 160Hz"
	if on {
		s.switchingTo = "1080p 320Hz"
	}
	s.notice = ""
	o := s.output
	go func() {
		err := link.SetCompetitive(on)
		s.post(func() {
			if err != nil {
				s.notice = locales.NoticeSwitchFailed()
			} else {
				s.notice = locales.NoticeSwitching(o.switchingTo)
			}
			o.switchingTo = ""
		})
	}()
}

func (s *store) sleepDisplay() { _ = display.SleepNow() }

// MARK: Lifecycle

// refreshDisplay reads the displays anew. Those still there keep their
// state; the panel stays on the one it showed while it is there.
func (s *store) refreshDisplay() {
	_, s.hasBuiltin = display.BuiltinBrightness()
	monitors := display.Targets()
	if len(monitors) == 0 {
		monitors = []display.Monitor{{}} // the panel still shows its software controls
	}
	outputs := make([]*output, 0, len(monitors))
	for _, m := range monitors {
		i := slices.IndexFunc(s.outputs, func(o *output) bool { return o.monitor.Key == m.Key })
		o := &output{lastBuiltin: -1}
		if i >= 0 {
			o = s.outputs[i]
		} else {
			d := s.prefs.display(m.Key)
			o.soft, o.warmth = d.SoftBrightness, d.Warmth
		}
		o.monitor, o.modes = m, m.Modes()
		o.current, o.hasCurrent = m.CurrentMode()
		outputs = append(outputs, o)
	}
	for _, o := range s.outputs {
		if !slices.Contains(outputs, o) && o.ddc != nil {
			go o.ddc.Close()
		}
	}
	s.outputs = outputs
	s.output = outputs[0]
	for _, o := range outputs {
		if o.monitor.Key == s.prefs.Display {
			s.output = o
		}
	}
}

// showDisplay makes o the display the panel shows.
func (s *store) showDisplay(o *output) {
	s.output, s.notice = o, ""
	s.prefs.Display, s.dirty = o.monitor.Key, true
}

// displaysChanged follows plugging, mode changes and wake: the gamma tables
// were reset, so capture them again and reapply.
func (s *store) displaysChanged() {
	previous := map[*output]display.Monitor{}
	for _, o := range s.outputs {
		previous[o] = o.monitor
	}
	s.refreshDisplay()
	display.ResetGammaBase()
	s.every(s.applyGamma)
	added := false
	for _, o := range s.outputs {
		m, ok := previous[o]
		if !ok || m != o.monitor {
			s.on(o, s.probeDDC)
		}
		added = added || !ok
	}
	if added {
		s.pairLinks()
	}
}

func (s *store) panelOpened() {
	s.usage = s.tracker.Recent(7)
	// Retry here too: the launch attempt may have raced the network
	// permission prompt.
	if s.link == nil && s.savedHost(s.output) != noLink {
		s.connect()
	} else {
		s.every(s.readVolume)
	}
	s.every(s.refreshDDC)
}

// detect looks for every control channel again.
func (s *store) detect() {
	s.connect()
	s.every(s.probeDDC)
}

// detectingAny reports a channel still being looked for.
func (s *store) detectingAny() bool { return s.detecting || s.probingDDC }

func (s *store) recordUsage() {
	s.autoCheck()
	awake := slices.ContainsFunc(s.outputs, func(o *output) bool { return !o.monitor.Asleep() })
	s.tracker.Record(awake, time.Duration(display.IdleSeconds()*float64(time.Second)))
	s.usage = s.tracker.Recent(7)
	s.save()
}

func (s *store) toggleLaunchAtLogin() {
	if err := mygo.App.SetOpenAtLogin(!s.launchAtLogin); err != nil {
		s.notice = locales.NoticeLoginFailed(err.Error())
	}
	s.launchAtLogin = mygo.App.OpenAtLogin()
}

func (s *store) save() {
	if !s.dirty {
		return
	}
	for i, o := range s.outputs {
		d := s.prefs.display(o.monitor.Key)
		d.SoftBrightness, d.Warmth = o.soft, o.warmth
		if i == 0 { // as before each display had its own
			s.prefs.SoftBrightness, s.prefs.Warmth = o.soft, o.warmth
		}
	}
	s.prefs.save()
	s.dirty = false
}
