//go:build windows

package keystate

import (
	"strconv"

	"golang.org/x/sys/windows"
)

var getAsyncKeyState = windows.NewLazySystemDLL("user32.dll").NewProc("GetAsyncKeyState")

func down(vk int) bool {
	r, _, _ := getAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

// codes are the virtual-key codes of the keys a shortcut can have, by the
// accelerator's names.
var codes = func() map[string][]int {
	m := map[string][]int{
		"Super": {0x5B, 0x5C}, "Ctrl": {0x11}, "Alt": {0x12}, "Shift": {0x10},
		"Up": {0x26}, "Down": {0x28}, "Left": {0x25}, "Right": {0x27},
		"Space": {0x20}, "Home": {0x24}, "End": {0x23}, "PageUp": {0x21}, "PageDown": {0x22},
		"-": {0xBD}, "=": {0xBB}, ",": {0xBC}, ".": {0xBE}, "/": {0xBF}, ";": {0xBA}, "'": {0xDE},
		"[": {0xDB}, "]": {0xDD}, `\`: {0xDC}, "`": {0xC0},
	}
	for i := range 26 {
		m[string(rune('A'+i))] = []int{'A' + i}
	}
	for i := range 10 {
		m[strconv.Itoa(i)] = []int{'0' + i}
	}
	for i := range 12 {
		m["F"+strconv.Itoa(i+1)] = []int{0x70 + i}
	}
	return m
}()
