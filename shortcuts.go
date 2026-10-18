package main

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// shortcut steps the brightness or the volume from anywhere (#9), with keys
// the user can record anew.
type shortcut struct {
	id, title string
	keys      string // the accelerator by default, such as "Ctrl+Alt+Up"
	run       func(s *store)
}

const shortcutStep = 5 // points per press

var shortcuts = []shortcut{
	{"brightnessUp", "亮度 +", "Ctrl+Alt+Up", func(s *store) { s.stepBrightness(shortcutStep) }},
	{"brightnessDown", "亮度 −", "Ctrl+Alt+Down", func(s *store) { s.stepBrightness(-shortcutStep) }},
	{"volumeUp", "音量 +", "Ctrl+Alt+Shift+Up", func(s *store) { s.stepVolume(shortcutStep) }},
	{"volumeDown", "音量 −", "Ctrl+Alt+Shift+Down", func(s *store) { s.stepVolume(-shortcutStep) }},
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
	for _, sc := range shortcuts {
		keys := s.prefs.keys(sc)
		if keys == "" {
			continue
		}
		err := mygo.GlobalShortcut.Register(keys, func() {
			sc.run(s)
			s.redraw()
		})
		if err != nil {
			s.notice = "快捷键 " + keysLabel(keys) + " 设置失败，可能已被其他应用占用"
			continue
		}
		s.registered[sc.id] = keys
	}
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
		return "未设置"
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
