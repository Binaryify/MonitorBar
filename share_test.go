package main

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/binaryify/MonitorBar/internal/display"
	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/internal/peer"
)

const (
	keyA27U = "25001:12300"
	keyGPro = "25001:10163"
)

// fakeSystem records what pull did to a pretend system.
type fakeSystem struct {
	screens    []display.Screen
	disabled   map[string]uint32
	enabled    []uint32
	lightOn    bool // an enabled screen comes back active
	selected   []int
	released   []string
	peerDown   bool
	peerAnswer peer.Release
}

func (f *fakeSystem) ops() shareOps {
	return shareOps{
		screens: func() []display.Screen { return f.screens },
		enable: func(id uint32) error {
			f.enabled = append(f.enabled, id)
			if f.lightOn {
				for i := range f.screens {
					if f.screens[i].ID == id {
						f.screens[i].Active = true
					}
				}
			}
			return nil
		},
		disabled: func() map[string]uint32 { return f.disabled },
		forget:   func(key string) { delete(f.disabled, key) },
		selectLAN: func(source int) error {
			f.selected = append(f.selected, source)
			return nil
		},
		release: func(key string, input int) (peer.Release, error) {
			if f.peerDown {
				return peer.Release{}, errors.New("ssh: connect timed out")
			}
			f.released = append(f.released, key+"/"+strconv.Itoa(input))
			return f.peerAnswer, nil
		},
		sleep: func(time.Duration) {},
	}
}

var (
	a27u = sharedScreen{Name: "A27U", Key: keyA27U, Channel: channelDDC, Input: 16}
	gpro = sharedScreen{Name: "G Pro", Key: keyGPro, Channel: channelLAN, Input: 29}
)

func TestPullDDCAsksThePeerToSwitch(t *testing.T) {
	f := &fakeSystem{
		screens:    []display.Screen{{ID: 3, Vendor: 25001, Model: 12300, Active: true}},
		disabled:   map[string]uint32{},
		peerAnswer: peer.Release{Held: true, Sent: true, Disabled: true},
	}
	got := pull(f.ops(), []sharedScreen{a27u})
	if got[0] != "A27U：已切到本机" || len(f.released) != 1 || f.released[0] != keyA27U+"/16" {
		t.Errorf("results %q, released %q", got, f.released)
	}
}

func TestPullTurnsTheScreenBackOnFirst(t *testing.T) {
	f := &fakeSystem{
		screens:    []display.Screen{{ID: 3, Vendor: 25001, Model: 12300}}, // given away earlier
		disabled:   map[string]uint32{keyA27U: 3},
		lightOn:    true,
		peerAnswer: peer.Release{Held: true, Sent: true, Disabled: true},
	}
	pull(f.ops(), []sharedScreen{a27u})
	if len(f.enabled) != 1 || f.enabled[0] != 3 {
		t.Errorf("enabled %v, want [3]", f.enabled)
	}
	if _, kept := f.disabled[keyA27U]; kept {
		t.Error("the screen is still remembered as given away")
	}
}

func TestPullWithoutLinkStops(t *testing.T) {
	f := &fakeSystem{disabled: map[string]uint32{}} // the cable's link never came up
	got := pull(f.ops(), []sharedScreen{a27u})
	if !strings.Contains(got[0], "没有这块屏的信号链路") || len(f.released) != 0 {
		t.Errorf("results %q, released %q", got, f.released)
	}
}

func TestPullLANSwitchesHereAndFreesThePeer(t *testing.T) {
	f := &fakeSystem{
		screens:  []display.Screen{{ID: 2, Vendor: 25001, Model: 10163, Active: true}},
		disabled: map[string]uint32{},
	}
	got := pull(f.ops(), []sharedScreen{gpro})
	if got[0] != "G Pro：已切到本机" || len(f.selected) != 1 || f.selected[0] != 29 || f.released[0] != keyGPro+"/0" {
		t.Errorf("results %q, selected %v, released %q", got, f.selected, f.released)
	}
}

func TestPullLANWithPeerDownStillSwitches(t *testing.T) {
	f := &fakeSystem{
		screens:  []display.Screen{{ID: 2, Vendor: 25001, Model: 10163, Active: true}},
		disabled: map[string]uint32{},
		peerDown: true,
	}
	got := pull(f.ops(), []sharedScreen{gpro})
	if !strings.Contains(got[0], "已切到本机（对方不在线") || len(f.selected) != 1 {
		t.Errorf("results %q, selected %v", got, f.selected)
	}
}

func TestPullDDCWithPeerDown(t *testing.T) {
	f := &fakeSystem{
		screens:  []display.Screen{{ID: 3, Vendor: 25001, Model: 12300, Active: true}},
		disabled: map[string]uint32{},
		peerDown: true,
	}
	if got := pull(f.ops(), []sharedScreen{a27u}); !strings.Contains(got[0], "对方不在线") {
		t.Errorf("results %q", got)
	}
}

func TestPullDDCNotOnThePeer(t *testing.T) {
	f := &fakeSystem{
		screens:    []display.Screen{{ID: 3, Vendor: 25001, Model: 12300, Active: true}},
		disabled:   map[string]uint32{},
		peerAnswer: peer.Release{},
	}
	if got := pull(f.ops(), []sharedScreen{a27u}); !strings.Contains(got[0], "应该已经在本机") {
		t.Errorf("results %q", got)
	}
}

// seedShare shares the A27U (here) and the G Pro (given to the other side).
func seedShare(p *panel) {
	s := p.s
	s.prefs.Tab, s.prefs.PeerHost = "共享", "mini.local"
	s.prefs.Shared = []sharedScreen{a27u, gpro}
	s.share.screens = []display.Screen{{ID: 3, Vendor: 25001, Model: 12300, Active: true}, {ID: 2, Vendor: 25001, Model: 10163}}
	s.share.disabled = map[string]uint32{keyGPro: 2}
	s.share.peer = &peer.Status{Host: "mini.local", Screens: []peer.Screen{{Key: keyGPro, Active: true}}}
	s.share.shortcutOn = defaultShortcut
}

func TestShareTab(t *testing.T) {
	p, _ := testPanel(t)
	seedShare(p)
	tt := newTester(p)
	for _, want := range []string{"切到本机", "全部切到本机", "快捷键 ⌃⌥⌘M", "A27U", "G Pro",
		"DDC/CI · 本机 DP 2 / USB-C · 本机排列中 · 对方已让出", "局域网 · 本机 DP · 已让给对方 · 对方排列中",
		"另一台电脑", "在线", "共享屏设置"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestOverviewShowsPullWhenShared(t *testing.T) {
	p, _ := testPanel(t)
	if tt := newTester(p); tt.HasText("全部切到本机") {
		t.Error("the pull card shows with nothing shared")
	}
	p.s.prefs.Shared = []sharedScreen{gpro}
	if tt := newTester(p); !tt.HasText("全部切到本机") {
		t.Error("the pull card is missing")
	}
}

func TestInputName(t *testing.T) {
	lan := []mitv.Source{{ID: 23, Name: "HDMI1"}, {ID: 29, Name: "DP"}}
	if got := inputName(gpro, lan); got != "DP" {
		t.Errorf("LAN input name %q", got)
	}
	if got := inputName(a27u, lan); got != "DP 2 / USB-C" {
		t.Errorf("DDC input name %q", got)
	}
	if got := shortcutLabel("Ctrl+Alt+Cmd+M"); got != "⌃⌥⌘M" {
		t.Errorf("shortcut label %q", got)
	}
}
