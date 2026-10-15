package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/peer"
)

// shareState is what the 共享 tab shows besides the settings.
type shareState struct {
	screens    []display.Screen  // every display connected here
	disabled   map[string]uint32 // the screens this computer gave away
	peer       *peer.Status      // the other computer, nil until it answered
	peerErr    string
	checking   bool
	pulling    bool
	results    []string // the last 切到本机, one line per screen
	resultsAt  time.Time
	shortcutOn string // the shortcut registered, "" when none
}

func (s *store) peerLink() peer.SSH { return peer.SSH{Host: s.prefs.PeerHost} }

// refreshScreens reads the displays connected here again.
func (s *store) refreshScreens() {
	s.share.screens = display.OnlineScreens()
	s.share.disabled = loadDisabled()
}

// checkPeer asks the other computer what it shows, in the background.
func (s *store) checkPeer() {
	if s.share.checking || s.prefs.PeerHost == "" {
		return
	}
	s.share.checking = true
	p := s.peerLink()
	go func() {
		st, err := p.Status()
		s.post(func() {
			s.share.checking = false
			if err != nil {
				s.share.peer, s.share.peerErr = nil, err.Error()
				return
			}
			s.share.peer, s.share.peerErr = &st, ""
		})
	}()
}

// pullAll brings every shared screen here; from names the trigger for
// the log.
func (s *store) pullAll(from string) {
	shareLog("pull all (%s)", from)
	s.pullScreens(s.prefs.Shared)
}

// pullScreen brings one shared screen here.
func (s *store) pullScreen(key string) {
	for _, sc := range s.prefs.Shared {
		if sc.Key == key {
			s.pullScreens([]sharedScreen{sc})
		}
	}
}

func (s *store) pullScreens(screens []sharedScreen) {
	if s.share.pulling || len(screens) == 0 {
		return
	}
	s.share.pulling, s.share.results = true, nil
	link := s.link
	if link == nil && s.prefs.NetworkHost != "" {
		link = mitv.New(s.prefs.NetworkHost) // found before: try it while discovery reruns
	}
	ops := systemOps(link, s.peerLink())
	screens = slices.Clone(screens)
	go func() {
		results := pull(ops, screens)
		shareLog("pull done: %s", strings.Join(results, " | "))
		s.post(func() {
			s.share.pulling, s.share.results, s.share.resultsAt = false, results, time.Now()
			s.refreshScreens()
			s.checkPeer()
		})
	}()
}

// screensChanged follows the displays changing here: the other computer
// may have just taken a screen away, so the last outcome no longer holds
// and the other side's state is asked again.
func (s *store) screensChanged() {
	s.refreshScreens()
	if !s.share.pulling && time.Since(s.share.resultsAt) > 3*time.Second {
		s.share.results = nil
	}
	s.checkPeer()
}

// location says which computer a shared screen is on, as far as the two
// arrangements tell: after 切到本机 only one of them keeps it. here is true
// when it is this computer.
func (s *store) location(key string) (text string, here bool) {
	local := false
	if sc, ok := display.FindScreen(s.share.screens, key); ok && sc.Active {
		local = true
	}
	if s.share.peer == nil { // the other side unknown: say what this side knows
		if local {
			return "本机已连接", true
		}
		if s.share.disabled[key] != 0 {
			return "已让给另一台", false
		}
		return "本机无链路", false
	}
	remote, name := s.share.peer.Has(key), s.share.peer.Label()
	switch {
	case local && !remote:
		return "在本机", true
	case remote && !local:
		return "在 " + name, false
	case local && remote:
		return "两台都连着", false
	}
	return "两台都未连接", false
}

// sharedIndex is the position of the screen in the settings, or -1.
func (s *store) sharedIndex(key string) int {
	return slices.IndexFunc(s.prefs.Shared, func(sc sharedScreen) bool { return sc.Key == key })
}

// setShared adds a connected screen to the shared ones, or removes it.
func (s *store) setShared(sc display.Screen, on bool) {
	i := s.sharedIndex(sc.Key())
	switch {
	case on && i < 0:
		entry := sharedScreen{Name: screenLabel(nil, sc), Key: sc.Key(), Channel: channelDDC, Input: ddcInputs[0].ID}
		if s.link != nil && len(s.inputs) > 0 {
			entry.Channel, entry.Input = channelLAN, s.inputs[0].ID
		}
		s.prefs.Shared = append(s.prefs.Shared, entry)
	case !on && i >= 0:
		s.prefs.Shared = slices.Delete(s.prefs.Shared, i, i+1)
	}
	s.dirty = true
}

// sharedChoices are the screens the settings list: those connected here,
// then those set before but not connected now.
func (s *store) sharedChoices() []display.Screen {
	var out []display.Screen
	for _, sc := range s.share.screens {
		if !sc.Builtin {
			out = append(out, sc)
		}
	}
	for _, sc := range s.prefs.Shared {
		if _, ok := display.FindScreen(out, sc.Key); !ok {
			var vendor, model uint32
			if n, _ := fmt.Sscanf(sc.Key, "%d:%d", &vendor, &model); n == 2 {
				out = append(out, display.Screen{Vendor: vendor, Model: model})
			}
		}
	}
	return out
}

// registerShortcut binds the chosen shortcut to 全部切到本机.
func (s *store) registerShortcut() {
	if s.share.shortcutOn != "" {
		mygo.GlobalShortcut.Unregister(s.share.shortcutOn)
		s.share.shortcutOn = ""
	}
	acc := s.prefs.PullShortcut
	if acc == "" || acc == "关闭" || len(s.prefs.Shared) == 0 {
		return
	}
	if err := mygo.GlobalShortcut.Register(acc, func() { s.pullAll("快捷键") }); err != nil {
		s.notice = "快捷键 " + acc + " 注册失败：" + err.Error()
		shareLog("shortcut %s failed: %v", acc, err)
		return
	}
	shareLog("shortcut %s registered", acc)
	s.share.shortcutOn = acc
}
