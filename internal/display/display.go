// Package display reads and changes the monitors through the operating
// system: their modes, gamma tables and whether they are lit.
// display_darwin.go and display_windows.go implement Targets and the
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

// Monitor is a display the app manages.
type Monitor struct {
	// Key tells the display from the others across launches: the same
	// for it whichever port, order or system id it has now.
	Key        string
	Name       string // e.g. "Mi Monitor"
	Connection string // e.g. "HDMI", or "" when unknown
	// Native is the panel's own resolution where the system tells it
	// (Windows, from the EDID), zero otherwise.
	NativeWidth, NativeHeight int
	Builtin                   bool // a MacBook's own display, when no external one is on
	order                     int  // its place among the displays, telling identical ones apart
	handle                    handle
}

// uniqueKeys suffixes the keys two displays share, as two monitors of a
// model whose EDID has no serial number do, in the order they come.
func uniqueKeys(ms []Monitor) []Monitor {
	seen := map[string]int{}
	for i := range ms {
		key := ms[i].Key
		if seen[key]++; seen[key] > 1 {
			ms[i].Key = fmt.Sprintf("%s#%d", key, seen[key])
		}
	}
	return ms
}

// NativeSize is the monitor's own resolution: what the system reports, or
// the largest mode drawn 1:1, as HiDPI modes draw more pixels than the
// panel has. A reported size smaller than the current mode is not trusted
// (virtual displays of remote desktops report one): the current mode wins.
func (m Monitor) NativeSize(modes []Mode, current Mode) (w, h int) {
	w, h = m.NativeWidth, m.NativeHeight
	if w == 0 {
		for _, mode := range modes {
			if !mode.HiDPI() && mode.PixelWidth > w {
				w, h = mode.PixelWidth, mode.PixelHeight
			}
		}
	}
	if !current.HiDPI() && current.PixelWidth > w {
		w, h = current.PixelWidth, current.PixelHeight
	}
	return w, h
}

// SizeLabel names a resolution the way monitors are sold: "4K", "2K".
func SizeLabel(w, h int) string {
	switch {
	case w >= 5120:
		return "5K"
	case w >= 3840:
		return "4K"
	case h >= 1440:
		return "2K"
	case h > 0:
		return fmt.Sprintf("%dp", h)
	}
	return ""
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
	return fmt.Sprintf("%dx%d@%dx%d", m.Width, m.Height, m.PixelWidth, m.PixelHeight)
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
		if a.Height != b.Height {
			return b.Height - a.Height
		}
		if a.PixelWidth != b.PixelWidth {
			return b.PixelWidth - a.PixelWidth
		}
		return b.PixelHeight - a.PixelHeight
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
