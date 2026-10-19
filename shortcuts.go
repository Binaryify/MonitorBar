package main

import (
	"maps"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/binaryify/MonitorBar/internal/mitv"
	"github.com/binaryify/MonitorBar/locales"
)

// shortcut steps the brightness or the volume from anywhere (#9), with keys
// the user can record anew.
type shortcut struct {
	id    string
	title func() string
	keys  string // the accelerator by default, such as "Ctrl+Alt+Up"
	run   func(s *store)
	hud   hudKind // what the HUD shows after it runs, "" for nothing
	hold  bool    // holding the keys repeats it
}

const shortcutStep = 5 // points per press

// Holding a stepping shortcut repeats it after a moment, as macOS repeats
// its own brightness and volume keys, until the keys go up.
const (
	holdDelay  = 400 * time.Millisecond
	holdRepeat = 80 * time.Millisecond // 0 to 100 in under 2 seconds
	holdPoll   = 15 * time.Millisecond
)

// The keys going down come first, as the left one of two neighbouring keys,
// such as F1 and F2, is the one recorded first.
var shortcuts = []shortcut{
	{"brightnessDown", locales.ShortcutBrightnessDown, "Ctrl+Alt+Down", func(s *store) { s.stepBrightness(-shortcutStep) }, hudBrightness, true},
	{"brightnessUp", locales.ShortcutBrightnessUp, "Ctrl+Alt+Up", func(s *store) { s.stepBrightness(shortcutStep) }, hudBrightness, true},
	{"volumeDown", locales.ShortcutVolumeDown, "Ctrl+Alt+Shift+Down", func(s *store) { s.stepVolume(-shortcutStep) }, hudVolume, true},
	{"volumeUp", locales.ShortcutVolumeUp, "Ctrl+Alt+Shift+Up", func(s *store) { s.stepVolume(shortcutStep) }, hudVolume, true},
	{"volumeMute", locales.ShortcutVolumeMute, "", func(s *store) { s.toggleMute() }, hudVolume, false},
}

// inputShortcut switches to an input, with no keys by default. It is kept
// by the input's id, so its keys stay while the display is away.
func inputShortcut(src mitv.Source) shortcut {
	return shortcut{id: inputShortcutPrefix + strconv.Itoa(src.ID), title: func() string { return locales.ShortcutInput(src.Name) },
		run: func(s *store) { s.switchInput(src.ID) }}
}

const inputShortcutPrefix = "input-"

// switchInput selects the input of id, when the display lists it.
func (s *store) switchInput(id int) {
	for _, src := range s.sources() {
		if src.ID == id {
			s.selectInput(src)
			return
		}
	}
}

// panelShortcuts are the shortcuts the panel lists: the fixed ones, and one
// per input the display lists.
func (s *store) panelShortcuts() []shortcut {
	list := slices.Clone(shortcuts)
	for _, src := range s.sources() {
		list = append(list, inputShortcut(src))
	}
	return list
}

// boundShortcuts are those to bind: the fixed ones, and the inputs' with
// keys recorded, also before the display's inputs are known.
func (s *store) boundShortcuts() []shortcut {
	list := slices.Clone(shortcuts)
	for _, id := range slices.Sorted(maps.Keys(s.prefs.Shortcuts)) {
		n, err := strconv.Atoi(strings.TrimPrefix(id, inputShortcutPrefix))
		if strings.HasPrefix(id, inputShortcutPrefix) && err == nil {
			list = append(list, inputShortcut(mitv.Source{ID: n}))
		}
	}
	return list
}

// keys is a shortcut's accelerator: the user's, else the default; "" when
// the user cleared it.
func (s *settings) keys(sc shortcut) string {
	if k, ok := s.Shortcuts[sc.id]; ok {
		return k
	}
	return sc.keys
}

// registerShortcuts binds the shortcuts again, after a change or while the
// panel records none: recording takes the keys for the panel.
func (s *store) registerShortcuts() {
	for id, keys := range s.registered {
		mygo.GlobalShortcut.Unregister(keys)
		delete(s.registered, id)
	}
	if s.recording != "" {
		return
	}
	for _, sc := range s.boundShortcuts() {
		keys := s.prefs.keys(sc)
		if keys == "" {
			continue
		}
		err := mygo.GlobalShortcut.Register(keys, func() { s.press(sc, keys) })
		if err != nil {
			s.notice = locales.ShortcutFailed(keysLabel(keys))
			continue
		}
		s.registered[sc.id] = keys
	}
}

// press runs a shortcut whose keys went down and, for one that holds,
// again and again while they stay down. Presses heard while it is held are
// the keyboard's repeat (Windows): the loop has taken over from them.
func (s *store) press(sc shortcut, keys string) {
	if s.held == sc.id {
		return
	}
	step := func() {
		sc.run(s)
		s.adjusted(sc.hud)
		s.redraw()
	}
	step()
	if !sc.hold {
		return
	}
	s.held = sc.id
	go func() {
		next := time.Now().Add(holdDelay)
		for s.keyHeld(keys) {
			if time.Now().After(next) {
				next = next.Add(holdRepeat)
				mygo.RunOnMain(step)
			}
			time.Sleep(holdPoll)
		}
		mygo.RunOnMain(func() {
			if s.held == sc.id {
				s.held = ""
			}
		})
	}()
}

// record starts recording a shortcut's keys, or stops with "".
func (s *store) record(id string) {
	if id == s.recording {
		return
	}
	s.recording = id
	if s.bindShortcuts {
		s.registerShortcuts()
	}
}

// setKeys keeps the keys recorded for a shortcut, "" for none.
func (s *store) setKeys(id, keys string) {
	if s.prefs.Shortcuts == nil {
		s.prefs.Shortcuts = map[string]string{}
	}
	s.prefs.Shortcuts[id], s.dirty = keys, true
	s.notice = ""
	s.record("")
}

// recordKey turns a key pressed while recording into an accelerator. It
// needs Ctrl, Option (Alt) or Command (Win), as a lone key would be taken
// from every app, except for a function key, which types nothing; ok is
// false for keys that can't make one.
func recordKey(mods ui.Modifiers, key ui.Key) (keys string, ok bool) {
	name, named := keyNames[key]
	fn := key >= ui.KeyF1 && key <= ui.KeyF12
	if !named || mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 && !fn {
		return "", false
	}
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Super, "Super"}, {ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}} {
		if mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, name), "+"), true
}

// keyNames are the keys a shortcut can end with, as accelerators name them.
var keyNames = func() map[ui.Key]string {
	names := map[ui.Key]string{
		ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyLeft: "Left", ui.KeyRight: "Right",
		ui.KeySpace: "Space", ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
		ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/",
		ui.KeySemicolon: ";", ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]",
		ui.KeyBackslash: `\`, ui.KeyBackquote: "`",
	}
	for i := range 12 { // the keys are numbered in order
		names[ui.KeyF1+ui.Key(i)] = "F" + strconv.Itoa(i+1)
	}
	for i := range 26 {
		names[ui.KeyA+ui.Key(i)] = string(rune('A' + i))
	}
	for i := range 10 {
		names[ui.Key0+ui.Key(i)] = string(rune('0' + i))
	}
	return names
}()

// keysLabel shows an accelerator the platform's way: ⌃⌥↑ on macOS,
// Ctrl+Alt+↑ elsewhere.
func keysLabel(keys string) string {
	if keys == "" {
		return locales.ShortcutNone()
	}
	arrows := map[string]string{"Up": "↑", "Down": "↓", "Left": "←", "Right": "→"}
	mac := map[string]string{"Super": "⌘", "Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧"}
	parts := strings.Split(keys, "+")
	for i, p := range parts {
		if a, ok := arrows[p]; ok {
			parts[i] = a
		} else if m, ok := mac[p]; ok && runtime.GOOS == "darwin" {
			parts[i] = m
		} else if p == "Super" {
			parts[i] = "Win"
		}
	}
	if runtime.GOOS == "darwin" {
		return strings.Join(parts, "")
	}
	return strings.Join(parts, "+")
}
