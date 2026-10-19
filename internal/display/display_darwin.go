//go:build darwin

package display

import (
	"errors"
	"os/exec"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// SettingsURL opens the Displays pane of System Settings.
const SettingsURL = "x-apple.systempreferences:com.apple.Displays-Settings.extension"

type (
	handle     = uint32  // CGDirectDisplayID
	nativeMode = uintptr // CGDisplayModeRef, retained
)

var (
	cgGetActiveDisplayList             func(max uint32, ids *uint32, count *uint32) int32
	cgMainDisplayID                    func() uint32
	cgDisplayIsBuiltin                 func(id uint32) uint32
	cgDisplayIsAsleep                  func(id uint32) uint32
	cgDisplayCopyAllDisplayModes       func(id uint32, options uintptr) uintptr
	cgDisplayCopyDisplayMode           func(id uint32) uintptr
	cgDisplayModeGetWidth              func(mode uintptr) uintptr
	cgDisplayModeGetHeight             func(mode uintptr) uintptr
	cgDisplayModeGetPixelWidth         func(mode uintptr) uintptr
	cgDisplayModeGetPixelHeight        func(mode uintptr) uintptr
	cgDisplayModeGetRefreshRate        func(mode uintptr) float64
	cgDisplayModeIsUsableForDesktopGUI func(mode uintptr) bool
	cgBeginDisplayConfiguration        func(config *uintptr) int32
	cgConfigureDisplayWithDisplayMode  func(config uintptr, id uint32, mode uintptr, options uintptr) int32
	cgCompleteDisplayConfiguration     func(config uintptr, option uint32) int32
	cgDisplayGammaTableCapacity        func(id uint32) uint32
	cgGetDisplayTransferByTable        func(id, capacity uint32, r, g, b *float32, count *uint32) int32
	cgSetDisplayTransferByTable        func(id, size uint32, r, g, b *float32) int32
	cgDisplayRestoreColorSyncSettings  func()
	cgEventSourceSecondsSinceLast      func(state int32, eventType uint32) float64
	cgDisplayVendorNumber              func(id uint32) uint32
	cgDisplayModelNumber               func(id uint32) uint32
	cgDisplaySerialNumber              func(id uint32) uint32

	cfArrayGetCount           func(array uintptr) int
	cfArrayGetValueAtIndex    func(array uintptr, i int) uintptr
	cfRetain                  func(ref uintptr) uintptr
	cfRelease                 func(ref uintptr)
	cfDictionaryCreate        func(alloc uintptr, keys, values *uintptr, n int, keyCallbacks, valueCallbacks uintptr) uintptr
	cfDictionaryGetValue      func(dict, key uintptr) uintptr
	cfStringCreateWithCString func(alloc uintptr, s string, encoding uint32) uintptr
	cfStringGetCString        func(s uintptr, buf *byte, size int, encoding uint32) bool
	cfGetTypeID               func(ref uintptr) uintptr
	cfStringGetTypeID         func() uintptr
	cfNumberGetValue          func(number uintptr, kind int32, value unsafe.Pointer) bool

	ioServiceMatching               func(name string) uintptr
	ioServiceGetMatchingServices    func(port uint32, matching uintptr, iter *uint32) int32
	ioIteratorNext                  func(iter uint32) uint32
	ioObjectRelease                 func(obj uint32) int32
	ioRegistryEntryCreateCFProperty func(entry uint32, key, alloc uintptr, options uint32) uintptr
	ioFBCopyI2CInterfaceForBus      func(fb, bus uint32, iface *uint32) int32
	ioI2CInterfaceOpen              func(iface, options uint32, connect *uintptr) int32 // IOI2CConnectRef is a pointer
	ioI2CInterfaceClose             func(connect uintptr, options uint32) int32
	ioI2CSendRequest                func(connect uintptr, options uint32, request *i2cRequest) int32
	ioDisplayCreateInfoDictionary   func(service, options uint32) uintptr

	modeOptions uintptr // {kCGDisplayShowDuplicateLowResolutionModes: true}, for HiDPI modes
	loadOnce    sync.Once
)

const utf8Encoding = 0x08000100

// cfNumberSInt32Type is CFNumberGetValue's kCFNumberSInt32Type.
const cfNumberSInt32Type = 3

func load() {
	loadOnce.Do(func() {
		lib := func(path string) uintptr {
			h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				panic(err)
			}
			return h
		}
		cg := lib("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		cf := lib("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		io := lib("/System/Library/Frameworks/IOKit.framework/IOKit")
		lib("/System/Library/Frameworks/AppKit.framework/AppKit")
		for fn, name := range map[any]string{
			&cgGetActiveDisplayList: "CGGetActiveDisplayList", &cgMainDisplayID: "CGMainDisplayID",
			&cgDisplayIsBuiltin: "CGDisplayIsBuiltin", &cgDisplayIsAsleep: "CGDisplayIsAsleep",
			&cgDisplayCopyAllDisplayModes: "CGDisplayCopyAllDisplayModes", &cgDisplayCopyDisplayMode: "CGDisplayCopyDisplayMode",
			&cgDisplayModeGetWidth: "CGDisplayModeGetWidth", &cgDisplayModeGetHeight: "CGDisplayModeGetHeight",
			&cgDisplayModeGetPixelWidth: "CGDisplayModeGetPixelWidth", &cgDisplayModeGetPixelHeight: "CGDisplayModeGetPixelHeight",
			&cgDisplayModeGetRefreshRate: "CGDisplayModeGetRefreshRate", &cgDisplayModeIsUsableForDesktopGUI: "CGDisplayModeIsUsableForDesktopGUI",
			&cgBeginDisplayConfiguration: "CGBeginDisplayConfiguration", &cgConfigureDisplayWithDisplayMode: "CGConfigureDisplayWithDisplayMode",
			&cgCompleteDisplayConfiguration: "CGCompleteDisplayConfiguration", &cgDisplayGammaTableCapacity: "CGDisplayGammaTableCapacity",
			&cgGetDisplayTransferByTable: "CGGetDisplayTransferByTable", &cgSetDisplayTransferByTable: "CGSetDisplayTransferByTable",
			&cgDisplayRestoreColorSyncSettings: "CGDisplayRestoreColorSyncSettings", &cgEventSourceSecondsSinceLast: "CGEventSourceSecondsSinceLastEventType",
			&cgDisplayVendorNumber: "CGDisplayVendorNumber", &cgDisplayModelNumber: "CGDisplayModelNumber",
			&cgDisplaySerialNumber: "CGDisplaySerialNumber",
		} {
			purego.RegisterLibFunc(fn, cg, name)
		}
		for fn, name := range map[any]string{
			&cfArrayGetCount: "CFArrayGetCount", &cfArrayGetValueAtIndex: "CFArrayGetValueAtIndex",
			&cfRetain: "CFRetain", &cfRelease: "CFRelease", &cfDictionaryCreate: "CFDictionaryCreate",
			&cfDictionaryGetValue: "CFDictionaryGetValue", &cfStringCreateWithCString: "CFStringCreateWithCString",
			&cfStringGetCString: "CFStringGetCString", &cfGetTypeID: "CFGetTypeID", &cfStringGetTypeID: "CFStringGetTypeID",
			&cfNumberGetValue: "CFNumberGetValue",
		} {
			purego.RegisterLibFunc(fn, cf, name)
		}
		for fn, name := range map[any]string{
			&ioServiceMatching: "IOServiceMatching", &ioServiceGetMatchingServices: "IOServiceGetMatchingServices",
			&ioIteratorNext: "IOIteratorNext", &ioObjectRelease: "IOObjectRelease",
			&ioRegistryEntryCreateCFProperty: "IORegistryEntryCreateCFProperty",
			&ioFBCopyI2CInterfaceForBus:      "IOFBCopyI2CInterfaceForBus",
			&ioI2CInterfaceOpen:              "IOI2CInterfaceOpen", &ioI2CInterfaceClose: "IOI2CInterfaceClose",
			&ioI2CSendRequest: "IOI2CSendRequest", &ioDisplayCreateInfoDictionary: "IODisplayCreateInfoDictionary",
		} {
			purego.RegisterLibFunc(fn, io, name)
		}
		key := symbolValue(cg, "kCGDisplayShowDuplicateLowResolutionModes")
		yes := symbolValue(cf, "kCFBooleanTrue")
		keyCallbacks, _ := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
		valueCallbacks, _ := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
		modeOptions = cfDictionaryCreate(0, &key, &yes, 1, keyCallbacks, valueCallbacks)
	})
}

// symbolValue reads a global pointer variable such as kCFBooleanTrue.
func symbolValue(lib uintptr, name string) uintptr {
	addr, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(err)
	}
	return **(**uintptr)(unsafe.Pointer(&addr))
}

// Target returns the first external display, or the main one.
func Target() Monitor {
	load()
	var ids [16]uint32
	var n uint32
	cgGetActiveDisplayList(uint32(len(ids)), &ids[0], &n)
	id := cgMainDisplayID()
	for _, d := range ids[:n] {
		if cgDisplayIsBuiltin(d) == 0 {
			id = d
			break
		}
	}
	return Monitor{Name: screenName(id), Connection: connection(), Builtin: cgDisplayIsBuiltin(id) != 0, handle: id}
}

// screenName asks NSScreen for the display's localized name.
func screenName(id uint32) string {
	sel := objc.RegisterName
	screens := objc.ID(objc.GetClass("NSScreen")).Send(sel("screens"))
	key := objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), "NSScreenNumber")
	for i := range objc.Send[uint64](screens, sel("count")) {
		screen := screens.Send(sel("objectAtIndex:"), i)
		number := screen.Send(sel("deviceDescription")).Send(sel("objectForKey:"), key)
		if objc.Send[uint32](number, sel("unsignedIntValue")) == id {
			return cString(objc.Send[*byte](screen.Send(sel("localizedName")), sel("UTF8String")))
		}
	}
	return "" // the panel names it
}

// connection reads the cable type ("HDMI", "DisplayPort") from IOKit.
func connection() string {
	var iter uint32
	if ioServiceGetMatchingServices(0, ioServiceMatching("IOPortTransportStateDisplayPort"), &iter) != 0 {
		return ""
	}
	defer ioObjectRelease(iter)
	metadata := cfStringCreateWithCString(0, "Metadata", utf8Encoding)
	defer cfRelease(metadata)
	field := cfStringCreateWithCString(0, "DFP Type Description", utf8Encoding)
	defer cfRelease(field)
	for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
		dict := ioRegistryEntryCreateCFProperty(entry, metadata, 0, 0)
		ioObjectRelease(entry)
		if dict == 0 {
			continue
		}
		value := goCFString(cfDictionaryGetValue(dict, field))
		cfRelease(dict)
		if value != "" {
			return value
		}
	}
	return ""
}

func (m Monitor) Modes() []Mode {
	load()
	array := cgDisplayCopyAllDisplayModes(m.handle, modeOptions)
	if array == 0 {
		return nil
	}
	defer cfRelease(array)
	var modes []Mode
	for i := range cfArrayGetCount(array) {
		ref := cfArrayGetValueAtIndex(array, i)
		if cgDisplayModeIsUsableForDesktopGUI(ref) {
			modes = append(modes, newMode(cfRetain(ref)))
		}
	}
	return dedupe(modes)
}

func (m Monitor) CurrentMode() (Mode, bool) {
	load()
	ref := cgDisplayCopyDisplayMode(m.handle)
	return newMode(ref), ref != 0
}

func newMode(ref uintptr) Mode {
	if ref == 0 {
		return Mode{}
	}
	rate := int(cgDisplayModeGetRefreshRate(ref) + 0.5)
	if rate == 0 {
		rate = 60
	}
	return Mode{
		Width: int(cgDisplayModeGetWidth(ref)), Height: int(cgDisplayModeGetHeight(ref)),
		PixelWidth: int(cgDisplayModeGetPixelWidth(ref)), PixelHeight: int(cgDisplayModeGetPixelHeight(ref)),
		RefreshRate: rate, native: ref,
	}
}

func (m Monitor) Apply(mode Mode) error {
	load()
	var config uintptr
	if cgBeginDisplayConfiguration(&config) != 0 {
		return errors.New("display: cannot begin configuration")
	}
	cgConfigureDisplayWithDisplayMode(config, m.handle, mode.native, 0)
	if cgCompleteDisplayConfiguration(config, 2 /* kCGConfigurePermanently */) != 0 {
		return errors.New("display: the display refused the mode")
	}
	return nil
}

var gammaBase [3][]float32 // the ColorSync curve, captured before the first change

// SetGamma dims and warms the picture through the gamma table. macOS resets
// it when the app quits.
func (m Monitor) SetGamma(brightness, warmth float64) error {
	load()
	if neutral(brightness, warmth) {
		RestoreGamma()
		return nil
	}
	if gammaBase[0] == nil {
		cgDisplayRestoreColorSyncSettings()
		capacity := cgDisplayGammaTableCapacity(m.handle)
		r, g, b := make([]float32, capacity), make([]float32, capacity), make([]float32, capacity)
		var n uint32
		cgGetDisplayTransferByTable(m.handle, capacity, &r[0], &g[0], &b[0], &n)
		gammaBase = [3][]float32{r[:n], g[:n], b[:n]}
	}
	if len(gammaBase[0]) == 0 {
		return errors.New("display: no gamma table")
	}
	gr, gg, gb := gains(brightness, warmth)
	scaled := [3][]float32{}
	for i, gain := range []float64{gr, gg, gb} {
		scaled[i] = make([]float32, len(gammaBase[i]))
		for j, v := range gammaBase[i] {
			scaled[i][j] = v * float32(gain)
		}
	}
	if cgSetDisplayTransferByTable(m.handle, uint32(len(scaled[0])), &scaled[0][0], &scaled[1][0], &scaled[2][0]) != 0 {
		return errors.New("display: the gamma table was refused")
	}
	return nil
}

// RestoreGamma puts the ColorSync curves back.
func RestoreGamma() {
	load()
	cgDisplayRestoreColorSyncSettings()
}

// ResetGammaBase forgets the captured curve, after the displays changed.
func ResetGammaBase() { gammaBase = [3][]float32{} }

func (m Monitor) Asleep() bool {
	load()
	return cgDisplayIsAsleep(m.handle) != 0
}

// IdleSeconds is the time since the last keyboard or mouse input.
func IdleSeconds() float64 {
	load()
	return cgEventSourceSecondsSinceLast(1 /* combined session */, 0xFFFFFFFF /* any input */)
}

// SleepNow turns the displays off until the next input.
func SleepNow() error { return exec.Command("/usr/bin/pmset", "displaysleepnow").Start() }

func goCFString(ref uintptr) string {
	if ref == 0 || cfGetTypeID(ref) != cfStringGetTypeID() {
		return ""
	}
	buf := make([]byte, 256)
	if !cfStringGetCString(ref, &buf[0], len(buf), utf8Encoding) {
		return ""
	}
	return cString(&buf[0])
}

func cString(p *byte) string {
	if p == nil {
		return ""
	}
	var b []byte
	for ptr := unsafe.Pointer(p); *(*byte)(ptr) != 0; ptr = unsafe.Add(ptr, 1) {
		b = append(b, *(*byte)(ptr))
	}
	return string(b)
}
