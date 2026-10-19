// Package mediakeys takes the keyboard's brightness and volume keys (F1, F2,
// F10, F11, F12 on a Mac without fn) from the system, for the external display.
// On macOS this takes an event tap, which needs the Accessibility
// permission; elsewhere it is not supported.
package mediakeys

import "errors"

// Key is a brightness or volume key.
type Key int

const (
	BrightnessUp Key = iota + 1
	BrightnessDown
	VolumeUp
	VolumeDown
	Mute
)

// Handler gets each press (down) and release of a key, on the main thread,
// and says whether to take it from the system. It must return quickly: the
// system holds every key event until it does.
type Handler func(k Key, down bool) (take bool)

// ErrNotTrusted is Start's error without the Accessibility permission.
var ErrNotTrusted = errors.New("mediakeys: no Accessibility permission")
