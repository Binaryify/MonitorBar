//go:build windows

package display

// BuiltinBrightness has no Windows counterpart yet: a desktop has no
// built-in display to follow.
func BuiltinBrightness() (float64, bool) { return 0, false }
