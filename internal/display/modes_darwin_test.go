//go:build darwin

package display

import (
	"math"
	"strings"
	"testing"
	"unsafe"
)

func saveModeFunc[T any](t *testing.T, at *T) {
	t.Helper()
	previous := *at
	t.Cleanup(func() { *at = previous })
}

type modeSystem struct {
	public        []Mode
	hidden        []cgsModeDescription
	current       Mode
	configured    int32
	publicRef     uintptr
	released      []uintptr
	cancelled     bool
	committed     bool
	beginCode     int32
	configureCode int32
	commitCode    int32
}

func stubModeSystem(t *testing.T) *modeSystem {
	t.Helper()
	loadCGS()
	saveModeFunc(t, &cgDisplayCopyAllDisplayModes)
	saveModeFunc(t, &cfArrayGetCount)
	saveModeFunc(t, &cfArrayGetValueAtIndex)
	saveModeFunc(t, &cfRelease)
	saveModeFunc(t, &cgDisplayModeIsUsableForDesktopGUI)
	saveModeFunc(t, &cgDisplayModeGetWidth)
	saveModeFunc(t, &cgDisplayModeGetHeight)
	saveModeFunc(t, &cgDisplayModeGetPixelWidth)
	saveModeFunc(t, &cgDisplayModeGetPixelHeight)
	saveModeFunc(t, &cgDisplayModeGetRefreshRate)
	saveModeFunc(t, &cgDisplayCopyDisplayMode)
	saveModeFunc(t, &cgsGetNumberOfDisplayModes)
	saveModeFunc(t, &cgsGetDisplayModeDescriptionOfLength)
	saveModeFunc(t, &cgsConfigureDisplayMode)
	saveModeFunc(t, &cgBeginDisplayConfiguration)
	saveModeFunc(t, &cgConfigureDisplayWithDisplayMode)
	saveModeFunc(t, &cgCompleteDisplayConfiguration)
	saveModeFunc(t, &cgCancelDisplayConfiguration)
	sys := &modeSystem{configured: -1}
	cgDisplayCopyAllDisplayModes = func(uint32, uintptr) uintptr {
		if len(sys.public) == 0 {
			return 0
		}
		return 100
	}
	cfArrayGetCount = func(uintptr) int { return len(sys.public) }
	cfArrayGetValueAtIndex = func(_ uintptr, i int) uintptr { return uintptr(i + 1) }
	cfRelease = func(ref uintptr) { sys.released = append(sys.released, ref) }
	value := func(ref uintptr) Mode {
		if ref == 200 {
			return sys.current
		}
		return sys.public[ref-1]
	}
	cgDisplayModeIsUsableForDesktopGUI = func(uintptr) bool { return true }
	cgDisplayModeGetWidth = func(ref uintptr) uintptr { return uintptr(value(ref).Width) }
	cgDisplayModeGetHeight = func(ref uintptr) uintptr { return uintptr(value(ref).Height) }
	cgDisplayModeGetPixelWidth = func(ref uintptr) uintptr { return uintptr(value(ref).PixelWidth) }
	cgDisplayModeGetPixelHeight = func(ref uintptr) uintptr { return uintptr(value(ref).PixelHeight) }
	cgDisplayModeGetRefreshRate = func(ref uintptr) float64 { return float64(value(ref).RefreshRate) }
	cgDisplayCopyDisplayMode = func(uint32) uintptr { return 200 }
	cgsGetNumberOfDisplayModes = func(_ uint32, count *int32) int32 { *count = int32(len(sys.hidden)); return 0 }
	cgsGetDisplayModeDescriptionOfLength = func(_ uint32, i int32, desc *cgsModeDescription, size int32) int32 {
		if size != 212 {
			t.Errorf("CGS descriptor size %d, want 212", size)
		}
		*desc = sys.hidden[i]
		return 0
	}
	cgsConfigureDisplayMode = func(_ uintptr, id uint32, mode int32) int32 {
		if id != 7 {
			t.Errorf("configured display %d, want 7", id)
		}
		sys.configured = mode
		return sys.configureCode
	}
	cgBeginDisplayConfiguration = func(config *uintptr) int32 { *config = 300; return sys.beginCode }
	cgConfigureDisplayWithDisplayMode = func(_ uintptr, id uint32, ref, _ uintptr) int32 {
		if id != 7 {
			t.Errorf("configured display %d, want 7", id)
		}
		sys.publicRef = ref
		return sys.configureCode
	}
	cgCompleteDisplayConfiguration = func(_ uintptr, option uint32) int32 {
		if option != 2 {
			t.Errorf("configuration option %d, want permanent", option)
		}
		sys.committed = true
		return sys.commitCode
	}
	cgCancelDisplayConfiguration = func(uintptr) int32 { sys.cancelled = true; return 0 }
	return sys
}

func TestCGSModeDiscovery(t *testing.T) {
	if unsafe.Sizeof(cgsModeDescription{}) != 212 || unsafe.Offsetof(cgsModeDescription{}.Density) != 208 || unsafe.Offsetof(cgsModeDescription{}.RefreshRate) != 190 {
		t.Fatal("CGS descriptor layout changed")
	}
	sys := stubModeSystem(t)
	valid := cgsModeDescription{Number: 42, Width: 2560, Height: 1440, Density: 2}
	sys.hidden = []cgsModeDescription{valid}
	for _, density := range []float32{0, 1, 5, float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		bad := valid
		bad.Density = density
		sys.hidden = append(sys.hidden, bad)
	}
	for _, change := range []func(*cgsModeDescription){
		func(d *cgsModeDescription) { d.Width = 0 },
		func(d *cgsModeDescription) { d.Height = 0 },
		func(d *cgsModeDescription) { d.Flags = 0x40000000 },
		func(d *cgsModeDescription) { d.Number = math.MaxUint32 },
	} {
		bad := valid
		change(&bad)
		sys.hidden = append(sys.hidden, bad)
	}
	m := Monitor{handle: 7}
	want := Mode{Width: 2560, Height: 1440, PixelWidth: 5120, PixelHeight: 2880, RefreshRate: 60}
	if got := m.Modes(); len(got) != 1 || got[0] != want {
		t.Fatalf("hidden discovery without a public array: %v", got)
	}
	sys.public = []Mode{want}
	if got := m.Modes(); len(got) != 1 || got[0] != want {
		t.Fatalf("public and hidden duplicates: %v", got)
	}
	cgsConfigureDisplayMode = nil
	if got := m.Modes(); len(got) != 1 || got[0] != want {
		t.Fatalf("public fallback with incomplete CGS symbols: %v", got)
	}
}

func TestModeApplication(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		public                   bool
		begin, configure, commit int32
		err                      string
	}{
		{name: "hidden mode is resolved again"},
		{name: "public mode takes precedence", public: true},
		{name: "begin failure", begin: 3, err: "begin"},
		{name: "configure failure cancels", configure: 4, err: "configure"},
		{name: "public configure failure cancels", public: true, configure: 4, err: "configure"},
		{name: "commit failure", commit: 5, err: "commit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sys := stubModeSystem(t)
			m := Monitor{handle: 7}
			sys.hidden = []cgsModeDescription{{Number: 42, Width: 2560, Height: 1440, RefreshRate: 120, Density: 2}}
			want := m.Modes()[0]
			sys.hidden[0].Number = 81
			if tc.public {
				sys.public = []Mode{want}
			}
			sys.current = want
			sys.beginCode, sys.configureCode, sys.commitCode = tc.begin, tc.configure, tc.commit
			err := m.Apply(want)
			if tc.err == "" && err != nil || tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)) {
				t.Fatalf("Apply: %v, want %q", err, tc.err)
			}
			if sys.cancelled != (tc.configure != 0) || sys.committed != (tc.begin == 0 && tc.configure == 0) {
				t.Fatalf("transaction cancellation %v, commit %v", sys.cancelled, sys.committed)
			}
			if tc.begin == 0 && (tc.public && sys.publicRef == 0 || !tc.public && sys.configured != 81) {
				t.Fatalf("native selection: public %d, CGS %d", sys.publicRef, sys.configured)
			}
		})
	}
}

func TestModeApplicationRejectsMissingOrUnusable(t *testing.T) {
	sys := stubModeSystem(t)
	want := Mode{Width: 1920, Height: 1080, PixelWidth: 3840, PixelHeight: 2160, RefreshRate: 60}
	sys.public = []Mode{want}
	cgDisplayModeIsUsableForDesktopGUI = func(uintptr) bool { return false }
	if err := (Monitor{handle: 7}).Apply(want); err == nil || sys.committed || sys.publicRef != 0 {
		t.Fatalf("unusable mode must not start a transaction: %v", err)
	}
	sys.public = nil
	if err := (Monitor{handle: 7}).Apply(want); err == nil {
		t.Fatal("a disappeared mode must be rejected")
	}
}

func TestModeApplicationVerifiesBackingAndRate(t *testing.T) {
	sys := stubModeSystem(t)
	want := Mode{Width: 2560, Height: 1440, PixelWidth: 5120, PixelHeight: 2880, RefreshRate: 120}
	sys.public = []Mode{want}
	sys.current = want
	sys.current.PixelWidth, sys.current.PixelHeight, sys.current.RefreshRate = 3840, 2160, 60
	if err := (Monitor{handle: 7}).Apply(want); err == nil || !strings.Contains(err.Error(), "did not take effect") {
		t.Fatalf("mismatched readback must not report success: %v", err)
	}
	if len(sys.released) < 2 || sys.released[len(sys.released)-1] != 100 {
		t.Fatalf("copied modes must be released: %v", sys.released)
	}
}

func TestCGSUnavailableKeepsPublicModes(t *testing.T) {
	for _, name := range []string{"missing count", "missing description", "missing configure", "negative count", "excessive count", "count error", "description error"} {
		t.Run(name, func(t *testing.T) {
			sys := stubModeSystem(t)
			public := Mode{Width: 1920, Height: 1080, PixelWidth: 1920, PixelHeight: 1080, RefreshRate: 60}
			sys.public = []Mode{public}
			sys.hidden = []cgsModeDescription{{Number: 42, Width: 2560, Height: 1440, Density: 2}}
			switch name {
			case "missing count":
				cgsGetNumberOfDisplayModes = nil
			case "missing description":
				cgsGetDisplayModeDescriptionOfLength = nil
			case "missing configure":
				cgsConfigureDisplayMode = nil
			case "negative count":
				cgsGetNumberOfDisplayModes = func(_ uint32, count *int32) int32 { *count = -1; return 0 }
			case "excessive count":
				cgsGetNumberOfDisplayModes = func(_ uint32, count *int32) int32 { *count = 4097; return 0 }
			case "count error":
				cgsGetNumberOfDisplayModes = func(uint32, *int32) int32 { return 1 }
			case "description error":
				cgsGetDisplayModeDescriptionOfLength = func(uint32, int32, *cgsModeDescription, int32) int32 { return 1 }
			}
			if got := (Monitor{handle: 7}).Modes(); len(got) != 1 || got[0] != public {
				t.Fatalf("public modes must survive unavailable CGS: %v", got)
			}
		})
	}
}
