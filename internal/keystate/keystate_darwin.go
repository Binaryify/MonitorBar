//go:build darwin

package keystate

import (
	"strconv"
	"sync"

	"github.com/ebitengine/purego"
)

var (
	keyState func(state int32, key uint16) bool
	loadOnce sync.Once
)

// down reads the key from the combined session state, which needs no
// permission.
func down(code int) bool {
	loadOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			purego.RegisterLibFunc(&keyState, cg, "CGEventSourceKeyState")
		}
	})
	const combinedSessionState = 0 // kCGEventSourceStateCombinedSessionState
	return keyState != nil && keyState(combinedSessionState, uint16(code))
}

// codes are the virtual key codes of the keys a shortcut can have (ANSI
// layout), by the accelerator's names.
var codes = func() map[string][]int {
	m := map[string][]int{
		"Super": {0x37, 0x36}, "Ctrl": {0x3B, 0x3E}, "Alt": {0x3A, 0x3D}, "Shift": {0x38, 0x3C},
		"Up": {0x7E}, "Down": {0x7D}, "Left": {0x7B}, "Right": {0x7C},
		"Space": {0x31}, "Home": {0x73}, "End": {0x77}, "PageUp": {0x74}, "PageDown": {0x79},
		"-": {0x1B}, "=": {0x18}, ",": {0x2B}, ".": {0x2F}, "/": {0x2C}, ";": {0x29}, "'": {0x27},
		"[": {0x21}, "]": {0x1E}, `\`: {0x2A}, "`": {0x32},
	}
	for i, c := range []int{0x00, 0x0B, 0x08, 0x02, 0x0E, 0x03, 0x05, 0x04, 0x22, 0x26, 0x28, 0x25, 0x2E,
		0x2D, 0x1F, 0x23, 0x0C, 0x0F, 0x01, 0x11, 0x20, 0x09, 0x0D, 0x07, 0x10, 0x06} { // A to Z
		m[string(rune('A'+i))] = []int{c}
	}
	for i, c := range []int{0x1D, 0x12, 0x13, 0x14, 0x15, 0x17, 0x16, 0x1A, 0x1C, 0x19} { // 0 to 9
		m[strconv.Itoa(i)] = []int{c}
	}
	for i, c := range []int{0x7A, 0x78, 0x63, 0x76, 0x60, 0x61, 0x62, 0x64, 0x65, 0x6D, 0x67, 0x6F} { // F1 to F12
		m["F"+strconv.Itoa(i+1)] = []int{c}
	}
	return m
}()
