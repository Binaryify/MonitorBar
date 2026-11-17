package display

import (
	"errors"
	"testing"
	"time"
)

type fakeLayout struct {
	state    int
	released bool
}

func (l *fakeLayout) Release() { l.released = true }

type fakeMirror struct {
	state, mirrors, restores              int
	failMirror, failRestore, failSnapshot bool
	snapshots                             []*fakeLayout
}

func (f *fakeMirror) Snapshot() (MirrorLayout, error) {
	if f.failSnapshot {
		return nil, errors.New("snapshot")
	}
	l := &fakeLayout{state: f.state}
	f.snapshots = append(f.snapshots, l)
	return l, nil
}
func (f *fakeMirror) Matches(l MirrorLayout) bool { return f.state == l.(*fakeLayout).state }
func (f *fakeMirror) Mirror() error {
	if f.failMirror {
		return ErrMirrorUnavailable
	}
	f.mirrors++
	f.state = 2
	return nil
}
func (f *fakeMirror) Restore(l MirrorLayout) error {
	if f.failRestore {
		return errors.New("restore")
	}
	f.restores++
	f.state = l.(*fakeLayout).state
	return nil
}
func controller() (*InputMirror, *fakeMirror) {
	f := &fakeMirror{state: 1}
	return &InputMirror{Backend: f, Mac: 0x11, Windows: 0x12}, f
}
func TestInputDebounceAndRestore(t *testing.T) {
	m, f := controller()
	now := time.Unix(100, 0)
	read := func(c int, ok bool) {
		t.Helper()
		now = now.Add(time.Second)
		if err := m.Observe(c, ok, now); err != nil {
			t.Fatal(err)
		}
	}
	read(0x12, true)
	read(0, false)
	read(0x12, true)
	read(0xf, true)
	read(0x12, true)
	if f.mirrors != 0 {
		t.Fatal("invalid readings counted")
	}
	read(0x12, true)
	if !m.Suspended() || f.mirrors != 1 {
		t.Fatal("no mirror")
	}
	read(0x11, true)
	read(0x11, true)
	if f.restores != 0 {
		t.Fatal("cooldown bypassed")
	}
	read(0x11, true)
	if m.Suspended() || f.state != 1 || f.restores != 1 {
		t.Fatal("no exact restore")
	}
	for _, l := range f.snapshots {
		if !l.released {
			t.Fatal("snapshot leaked")
		}
	}
}
func TestMirrorManualOwnership(t *testing.T) {
	m, f := controller()
	now := time.Unix(100, 0)
	m.Observe(0x12, true, now)
	m.Observe(0x12, true, now.Add(time.Second))
	f.state = 3 // user made a new layout
	for i := 2; i < 10; i++ {
		m.Observe(0x12, true, now.Add(time.Duration(i)*time.Second))
	}
	if f.state != 3 || f.mirrors != 1 {
		t.Fatal("unchanged input overrode manual layout")
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if f.restores != 0 || f.state != 3 {
		t.Fatal("restored unowned layout")
	}
}
func TestMirrorFailuresAndRetryRestore(t *testing.T) {
	m, f := controller()
	f.failMirror = true
	if m.Arm() == nil || m.Suspended() {
		t.Fatal("claimed failed mutation")
	}
	if !f.snapshots[0].released {
		t.Fatal("leaked failed snapshot")
	}
	f.failMirror = false
	if err := m.Arm(); err != nil {
		t.Fatal(err)
	}
	f.failRestore = true
	if m.Stop() == nil || !m.Suspended() {
		t.Fatal("lost failed restore ownership")
	}
	f.failRestore = false
	if err := m.Stop(); err != nil || f.state != 1 {
		t.Fatal("restore retry failed")
	}
}
func TestInputCodesNotTransport(t *testing.T) {
	m, f := controller()
	now := time.Now()
	for i, c := range []int{0xf, 0xf, 0x10, 0x10, 0, 0x13, 0x13} {
		m.Observe(c, true, now.Add(time.Duration(i)*time.Second))
	}
	if f.mirrors != 0 {
		t.Fatal("transport or unknown input inferred Windows")
	}
	m.Mac = m.Windows
	m.Observe(0x12, true, now)
	m.Observe(0x12, true, now.Add(time.Second))
	if f.mirrors != 0 {
		t.Fatal("ambiguous mapping accepted")
	}
}
func TestInitialMacAndNoopStopPreserveLayout(t *testing.T) {
	m, f := controller()
	now := time.Now()
	m.Observe(0x11, true, now)
	m.Observe(0x11, true, now.Add(time.Second))
	m.Stop()
	if f.restores != 0 || f.mirrors != 0 {
		t.Fatal("changed initial manual layout")
	}
}

func TestPrearmIgnoresStaleMacUntilRequested(t *testing.T) {
	m, f := controller()
	now := time.Now()
	m.Observe(0x11, true, now)
	m.Observe(0x11, true, now.Add(time.Second))
	if err := m.Prearm(); err != nil {
		t.Fatal(err)
	}
	m.Observe(0x11, true, now.Add(2*time.Second))
	if !m.Suspended() {
		t.Fatal("restored before confirmation")
	}
	m.Observe(0x11, true, now.Add(3*time.Second))
	if !m.Suspended() {
		t.Fatal("stale Mac reply undid Windows mirroring")
	}
	m.RequestMac()
	m.Observe(0x11, true, now.Add(4*time.Second))
	m.Observe(0x11, true, now.Add(5*time.Second))
	if m.Suspended() || f.state != 1 {
		t.Fatal("explicit Mac request did not restore")
	}
}

func TestFailedRestoreRetriesOnValidMacOnly(t *testing.T) {
	m, f := controller()
	now := time.Now()
	if err := m.Arm(); err != nil {
		t.Fatal(err)
	}
	f.failRestore = true
	m.Observe(0x11, true, now)
	if m.Observe(0x11, true, now.Add(time.Second)) == nil {
		t.Fatal("expected restore failure")
	}
	f.failRestore = false
	m.Observe(0x11, false, now.Add(5*time.Second))
	if !m.Suspended() {
		t.Fatal("failed reading restored")
	}
	m.Observe(0x11, true, now.Add(6*time.Second))
	m.Observe(0x11, true, now.Add(7*time.Second))
	if m.Suspended() || f.state != 1 {
		t.Fatal("valid retry did not restore")
	}
}

func TestExplicitSwitchRearmsAfterManualExtendedLayout(t *testing.T) {
	m, f := controller()
	if err := m.Arm(); err != nil {
		t.Fatal(err)
	}
	f.state = 3
	if err := m.Prearm(); err != nil {
		t.Fatal(err)
	}
	if f.mirrors != 2 || f.restores != 0 {
		t.Fatal("did not safely rearm new layout")
	}
	if err := m.Stop(); err != nil || f.state != 3 {
		t.Fatal("did not preserve new manual baseline")
	}
}

func TestWindowsConfirmationPreservesManualLayoutAfterPrearm(t *testing.T) {
	m, f := controller()
	now := time.Now()
	if err := m.Prearm(); err != nil {
		t.Fatal(err)
	}
	f.state = 3 // user changed the layout before Windows confirmation arrived
	m.Observe(0x12, true, now)
	m.Observe(0x12, true, now.Add(time.Second))
	if f.state != 3 || f.mirrors != 1 {
		t.Fatal("confirmation overrode manual layout")
	}
	m.Stop()
	if f.state != 3 || f.restores != 0 {
		t.Fatal("restored over external ownership")
	}
}
