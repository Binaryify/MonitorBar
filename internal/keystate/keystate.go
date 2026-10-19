// Package keystate tells whether the keys of a shortcut are still held, for
// repeating it while they are, as the system repeats its own keys. Global
// shortcuts hear only the press: macOS's never repeat, Windows's repeat at
// the keyboard's rate.
package keystate

import "strings"

// Held says whether every key of an accelerator, such as "Ctrl+Alt+Up", is
// down; false for keys it can't read, and where it can't read any.
func Held(keys string) bool {
	if keys == "" {
		return false
	}
	for _, name := range strings.Split(keys, "+") {
		codes, ok := codes[name]
		if !ok || !anyDown(codes) {
			return false
		}
	}
	return true
}

// anyDown says whether one of a key's codes is down: the left or right of
// a modifier.
func anyDown(codes []int) bool {
	for _, c := range codes {
		if down(c) {
			return true
		}
	}
	return false
}
