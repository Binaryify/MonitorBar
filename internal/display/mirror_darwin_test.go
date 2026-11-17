//go:build darwin

package display

import "testing"

// Replace every mutating CoreGraphics entry point: these tests never configure
// a physical display or require an attached monitor.
func TestNativeRestoreTransaction(t *testing.T) {
	begin, mirror, mode, origin, complete, cancel := cgBeginDisplayConfiguration, cgMirror, cgConfigureDisplayWithDisplayMode, cgOrigin, cgCompleteDisplayConfiguration, cgCancel
	defer func() {
		cgBeginDisplayConfiguration, cgMirror, cgConfigureDisplayWithDisplayMode, cgOrigin, cgCompleteDisplayConfiguration, cgCancel = begin, mirror, mode, origin, complete, cancel
	}()
	var mirrors [][2]uint32
	var modes []uintptr
	var origins [][3]int32
	completed, cancelled := 0, 0
	cgBeginDisplayConfiguration = func(c *uintptr) int32 { *c = 99; return 0 }
	cgMirror = func(c uintptr, id, master uint32) int32 { mirrors = append(mirrors, [2]uint32{id, master}); return 0 }
	cgConfigureDisplayWithDisplayMode = func(c uintptr, id uint32, mode, options uintptr) int32 { modes = append(modes, mode); return 0 }
	cgOrigin = func(c uintptr, id uint32, x, y int32) int32 {
		origins = append(origins, [3]int32{int32(id), x, y})
		return 0
	}
	cgCompleteDisplayConfiguration = func(c uintptr, option uint32) int32 {
		if c != 99 || option != 0 {
			t.Fatal("not app-scoped")
		}
		completed++
		return 0
	}
	cgCancel = func(c uintptr) int32 { cancelled++; return 0 }
	layout := macLayout{{id: 1, mode: 100, x: 0, y: 0}, {id: 2, mode: 200, x: -2560, y: 120}}
	if err := (macMirror{2}).Restore(layout); err != nil {
		t.Fatal(err)
	}
	if len(mirrors) != 2 || mirrors[1] != [2]uint32{2, 0} || len(modes) != 2 || modes[0] != 100 || modes[1] != 200 || origins[1] != [3]int32{2, -2560, 120} || completed != 1 || cancelled != 0 {
		t.Fatalf("inexact restore: %v %v %v", mirrors, modes, origins)
	}
	cgOrigin = func(uintptr, uint32, int32, int32) int32 { return 7 }
	if (macMirror{2}).Restore(layout) == nil || cancelled != 1 || completed != 1 {
		t.Fatal("failed transaction was not cancelled")
	}
}
