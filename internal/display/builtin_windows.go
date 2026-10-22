//go:build windows

package display

// BuiltinBrightness has no Windows counterpart yet: following a laptop's
// own panel is macOS only.
func BuiltinBrightness() (float64, bool) { return 0, false }
