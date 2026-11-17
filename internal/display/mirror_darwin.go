//go:build darwin

package display

import (
	"fmt"
	"github.com/ebitengine/purego"
)

type cgPoint struct{ X, Y float64 }
type cgSize struct{ W, H float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}

var (
	cgOnline   func(uint32, *uint32, *uint32) int32
	cgMirrorOf func(uint32) uint32
	cgBounds   func(uint32) cgRect
	cgOrigin   func(uintptr, uint32, int32, int32) int32
	cgMirror   func(uintptr, uint32, uint32) int32
	cgCancel   func(uintptr) int32
	cgModeID   func(uintptr) uint32
)

func registerMirror(cg uintptr) {
	for fn, name := range map[any]string{
		&cgOnline: "CGGetOnlineDisplayList", &cgMirrorOf: "CGDisplayMirrorsDisplay",
		&cgBounds: "CGDisplayBounds", &cgOrigin: "CGConfigureDisplayOrigin",
		&cgMirror: "CGConfigureDisplayMirrorOfDisplay", &cgCancel: "CGCancelDisplayConfiguration",
		&cgModeID: "CGDisplayModeGetIODisplayModeID",
	} {
		purego.RegisterLibFunc(fn, cg, name)
	}
}

type layoutDisplay struct {
	id, mirror uint32
	x, y       int32
	mode       uintptr
	modeID     uint32
}
type macLayout []layoutDisplay

func (l macLayout) Release() {
	for _, d := range l {
		if d.mode != 0 {
			cfRelease(d.mode)
		}
	}
}

type macMirror struct{ target uint32 }

func onlineIDs() ([]uint32, error) {
	var ids [64]uint32
	var n uint32
	if cgOnline(uint32(len(ids)), &ids[0], &n) != 0 || n > uint32(len(ids)) {
		return nil, ErrMirrorUnavailable
	}
	return ids[:n], nil
}
func (m Monitor) Mirrored() bool {
	if m.handle == 0 {
		return false
	}
	load()
	return cgMirrorOf(m.handle) != 0
}
func (m Monitor) InputMirror(mac, windows int) *InputMirror {
	load()
	if m.Builtin {
		return nil
	}
	return &InputMirror{Backend: macMirror{m.handle}, Mac: mac, Windows: windows}
}
func (b macMirror) Snapshot() (MirrorLayout, error) {
	ids, err := onlineIDs()
	if err != nil {
		return nil, err
	}
	l := macLayout{}
	for _, id := range ids {
		mode := cgDisplayCopyDisplayMode(id)
		if mode == 0 {
			l.Release()
			return nil, ErrMirrorUnavailable
		}
		r := cgBounds(id)
		l = append(l, layoutDisplay{id: id, mirror: cgMirrorOf(id), x: int32(r.Origin.X), y: int32(r.Origin.Y), mode: mode, modeID: cgModeID(mode)})
	}
	return l, nil
}
func (b macMirror) Matches(snapshot MirrorLayout) bool {
	now, err := b.Snapshot()
	if err != nil {
		return false
	}
	defer now.Release()
	a, c := snapshot.(macLayout), now.(macLayout)
	if len(a) != len(c) {
		return false
	}
	for _, d := range a {
		found := false
		for _, e := range c {
			if d.id == e.id {
				found = d.mirror == e.mirror && d.x == e.x && d.y == e.y && d.modeID == e.modeID
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func configure(fn func(uintptr) int32) error {
	var c uintptr
	if cgBeginDisplayConfiguration(&c) != 0 {
		return ErrMirrorUnavailable
	}
	if e := fn(c); e != 0 {
		cgCancel(c)
		return fmt.Errorf("display: configuration failed (%d)", e)
	}
	if e := cgCompleteDisplayConfiguration(c, 0 /* app only, never permanent */); e != 0 {
		return fmt.Errorf("display: completion failed (%d)", e)
	}
	return nil
}
func (b macMirror) Mirror() error {
	ids, err := onlineIDs()
	if err != nil {
		return err
	}
	var builtin uint32
	found := false
	for _, id := range ids {
		// Never claim a preexisting mirror set, including one belonging to others.
		if cgMirrorOf(id) != 0 {
			return ErrMirrorUnavailable
		}
		if id == b.target {
			found = true
		}
		if cgDisplayIsBuiltin(id) != 0 {
			builtin = id
		}
	}
	if builtin == 0 || !found || cgMainDisplayID() != builtin {
		return ErrMirrorUnavailable
	}
	// Clear only the external curve before joining the built-in mirror set.
	if err := (Monitor{handle: b.target}).SetGamma(1, 0); err != nil {
		return err
	}
	return configure(func(c uintptr) int32 { return cgMirror(c, b.target, builtin) })
}
func (b macMirror) Restore(snapshot MirrorLayout) error {
	return configure(func(c uintptr) int32 {
		for _, d := range snapshot.(macLayout) {
			if e := cgMirror(c, d.id, d.mirror); e != 0 {
				return e
			}
		}
		for _, d := range snapshot.(macLayout) {
			if e := cgConfigureDisplayWithDisplayMode(c, d.id, d.mode, 0); e != 0 {
				return e
			}
			if e := cgOrigin(c, d.id, d.x, d.y); e != 0 {
				return e
			}
		}
		return 0
	})
}
