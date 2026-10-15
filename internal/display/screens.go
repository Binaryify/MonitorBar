package display

import (
	"errors"
	"fmt"
)

// ErrUnsupported means the platform cannot do this (yet).
var ErrUnsupported = errors.New("display: not supported on this platform")

// Screen is one display the system knows, active or not. Shared screens
// (one monitor wired to two computers) are told apart by their EDID vendor
// and model numbers, which stay the same across cables and reboots, while
// the display ID may change.
type Screen struct {
	ID      uint32 // the system's display ID (CGDirectDisplayID on macOS)
	Vendor  uint32
	Model   uint32
	Builtin bool
	Active  bool // drawn to: a disabled display is online but not active
	Main    bool
}

// Key identifies a monitor model: "25001:12300".
func (s Screen) Key() string { return ScreenKey(s.Vendor, s.Model) }

// ScreenKey formats a vendor and model as Screen.Key does.
func ScreenKey(vendor, model uint32) string { return fmt.Sprintf("%d:%d", vendor, model) }

// FindScreen returns the online screen with the key.
func FindScreen(screens []Screen, key string) (Screen, bool) {
	for _, s := range screens {
		if s.Key() == key {
			return s, true
		}
	}
	return Screen{}, false
}
