// Package display reads and changes the target monitor through the
// operating system: its modes, its gamma table and whether it is lit.
// display_darwin.go and display_windows.go implement Target and the
// Monitor methods.
package display

import (
	"errors"
	"fmt"
	"slices"
)

// ErrGammaLimited means the system refused the requested dimming and a
// milder one was applied: Windows keeps gamma ramps near the identity.
var ErrGammaLimited = errors.New("display: the system limited the dimming")

// Monitor is the external display the app manages.
type Monitor struct {
	Name       string // e.g. "Mi Monitor"
	Connection string // e.g. "HDMI", or "" when unknown
	handle     handle
}

// Mode is a resolution and refresh rate the monitor can show.
type Mode struct {
	Width, Height           int // the size the desktop uses (points on macOS)
	PixelWidth, PixelHeight int // the size drawn
	RefreshRate             int
	native                  nativeMode
}

func (m Mode) HiDPI() bool { return m.PixelWidth > m.Width }

// SizeKey identifies the resolution, whatever the refresh rate.
func (m Mode) SizeKey() string {
	return fmt.Sprintf("%dx%d@%v", m.Width, m.Height, m.HiDPI())
}

func (m Mode) ID() string    { return fmt.Sprintf("%s/%d", m.SizeKey(), m.RefreshRate) }
func (m Mode) Title() string { return fmt.Sprintf("%d × %d", m.Width, m.Height) }

// Resolutions returns one mode per resolution, HiDPI or not, preferring the
// given refresh rate, from the largest.
func Resolutions(modes []Mode, hiDPI bool, refreshRate int) []Mode {
	best := map[string]Mode{}
	for _, m := range modes {
		if m.HiDPI() != hiDPI {
			continue
		}
		cur, ok := best[m.SizeKey()]
		if !ok || cur.RefreshRate != refreshRate && (m.RefreshRate == refreshRate || m.RefreshRate > cur.RefreshRate) {
			best[m.SizeKey()] = m
		}
	}
	out := make([]Mode, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b Mode) int {
		if a.Width != b.Width {
			return b.Width - a.Width
		}
		return b.Height - a.Height
	})
	return out
}

// RefreshRates returns the modes of the resolution of current, fastest first.
func RefreshRates(modes []Mode, current Mode) []Mode {
	var out []Mode
	for _, m := range modes {
		if m.SizeKey() == current.SizeKey() {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b Mode) int { return b.RefreshRate - a.RefreshRate })
	return out
}

// dedupe keeps the first mode of each ID: systems list the same mode once
// per pixel encoding.
func dedupe(modes []Mode) []Mode {
	seen := map[string]bool{}
	return slices.DeleteFunc(modes, func(m Mode) bool {
		if seen[m.ID()] {
			return true
		}
		seen[m.ID()] = true
		return false
	})
}

// gains scales the color channels: brightness 0...1 dims all of them,
// warmth 0...1 cuts green a little and blue more.
func gains(brightness, warmth float64) (r, g, b float64) {
	return brightness, brightness * (1 - 0.2*warmth), brightness * (1 - 0.55*warmth)
}

// neutral reports settings that leave the display untouched.
func neutral(brightness, warmth float64) bool { return brightness >= 0.999 && warmth <= 0.001 }
