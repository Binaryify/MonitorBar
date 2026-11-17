package display

import (
	"errors"
	"time"
)

// MirrorLayout is an owned, retained snapshot; callers must release it.
type MirrorLayout interface{ Release() }
type MirrorBackend interface {
	Snapshot() (MirrorLayout, error)
	Matches(MirrorLayout) bool
	Mirror() error
	Restore(MirrorLayout) error
}

// InputMirror only changes layouts on actual, debounced physical source
// transitions. Transport names (including DisplayPort through docks) play no role.
type InputMirror struct {
	Backend                  MirrorBackend
	Mac, Windows             int
	candidate, count, stable int
	last                     time.Time
	saved, owned             MirrorLayout
	restorePending           bool
	waitingWindows           bool
}

func (m *InputMirror) Suspended() bool { return m.owned != nil }

// OwnsLayout checks whether the current layout still belongs to this controller.
func (m *InputMirror) OwnsLayout() bool { return m.owned != nil && m.Backend.Matches(m.owned) }
func (m *InputMirror) Observe(code int, valid bool, now time.Time) error {
	if !valid || (code != m.Mac && code != m.Windows) || m.Mac == m.Windows {
		m.candidate, m.count = 0, 0
		return nil
	}
	// Docks may keep returning stale Mac-source replies after a Windows switch.
	// Do not restore until Windows has been observed, or the user requests Mac.
	if m.waitingWindows && code == m.Mac {
		m.candidate, m.count = 0, 0
		return nil
	}
	if code != m.candidate {
		m.candidate, m.count = code, 1
	} else {
		m.count++
	}
	if m.count < 2 || (code == m.stable && !(code == m.Mac && m.restorePending)) || (!m.last.IsZero() && now.Sub(m.last) < 3*time.Second) {
		return nil
	}
	m.stable, m.last = code, now
	if code == m.Windows {
		m.waitingWindows = false
		return m.Arm()
	}
	return m.Stop()
}
func (m *InputMirror) Arm() error {
	if m.Mac <= 0 || m.Windows <= 0 || m.Mac == m.Windows {
		return ErrMirrorUnavailable
	}
	if m.owned != nil {
		return nil
	}
	before, err := m.Backend.Snapshot()
	if err != nil {
		return err
	}
	if err = m.Backend.Mirror(); err != nil {
		before.Release()
		return err
	}
	after, err := m.Backend.Snapshot()
	if err != nil {
		_ = m.Backend.Restore(before)
		before.Release()
		return err
	}
	m.saved, m.owned = before, after
	return nil
}
func (m *InputMirror) Stop() error {
	if m.owned == nil {
		return nil
	}
	// An external layout change revokes ownership; do not undo it.
	if !m.Backend.Matches(m.owned) {
		m.release()
		return nil
	}
	if err := m.Backend.Restore(m.saved); err != nil {
		m.restorePending = true
		return err
	}
	m.release()
	return nil
}
func (m *InputMirror) release() {
	m.saved.Release()
	m.owned.Release()
	m.saved, m.owned = nil, nil
	m.restorePending = false
}

var ErrMirrorUnavailable = errors.New("display: input mirroring unavailable or layout already mirrored")

// Prearm waits for confirmed Windows before accepting an automatic Mac return.
// A dock can report stale Mac replies even after a successful Windows switch.
func (m *InputMirror) Prearm() error {
	// Only an explicit request may reclaim a manually changed layout.
	if m.owned != nil && !m.Backend.Matches(m.owned) {
		m.release()
	}
	if err := m.Arm(); err != nil {
		return err
	}
	m.stable, m.candidate, m.count = 0, 0, 0
	m.waitingWindows = true
	m.last = time.Time{}
	return nil
}

// RequestMac authorizes a fresh return confirmation even on docks that cannot
// report Windows while the Mac video cable is inactive.
func (m *InputMirror) RequestMac() {
	m.waitingWindows = false
	m.stable, m.candidate, m.count = 0, 0, 0
	m.last = time.Time{}
}
