//go:build darwin

package mediakeys

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Supported says whether this system can take the keys.
const Supported = true

// SettingsURL opens Privacy & Security › Accessibility in System Settings.
const SettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

const (
	sessionTap       = 1 // kCGSessionEventTap
	headInsert       = 0 // kCGHeadInsertEventTap
	defaultTap       = 0 // kCGEventTapOptionDefault: may take events
	keyDown          = 10
	keyUp            = 11
	systemDefined    = 14 // NX_SYSDEFINED, which carries the media keys
	disabledTimeout  = 0xFFFFFFFE
	disabledByInput  = 0xFFFFFFFF
	auxControlButton = 8 // NX_SUBTYPE_AUX_CONTROL_BUTTONS: the media keys
	keyStateDown     = 0xA
)

var (
	axIsProcessTrusted            func() uint8
	axIsProcessTrustedWithOptions func(options uintptr) uint8
	cgEventTapCreate              func(tap, place, options uint32, mask uint64, callback, info uintptr) uintptr
	cgEventTapEnable              func(tap uintptr, enable bool)
	cgEventGetIntegerValueField   func(event uintptr, field uint32) int64
	cfMachPortCreateRunLoopSource func(alloc, port uintptr, order int) uintptr
	cfMachPortInvalidate          func(port uintptr)
	cfRunLoopGetMain              func() uintptr
	cfRunLoopAddSource            func(loop, source, mode uintptr)
	cfRunLoopRemoveSource         func(loop, source, mode uintptr)
	cfRelease                     func(ref uintptr)
	cfDictionaryCreate            func(alloc uintptr, keys, values *uintptr, n int, keyCallbacks, valueCallbacks uintptr) uintptr

	commonModes   uintptr // kCFRunLoopCommonModes
	promptOptions uintptr // {kAXTrustedCheckOptionPrompt: true}
	callback      uintptr
	loadOnce      sync.Once

	// The tap while running, on the main thread.
	tap, source uintptr
	handler     Handler
)

func load() {
	loadOnce.Do(func() {
		lib := func(path string) uintptr {
			h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				panic(err)
			}
			return h
		}
		ax := lib("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices")
		cg := lib("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		cf := lib("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		lib("/System/Library/Frameworks/AppKit.framework/AppKit") // NSEvent
		purego.RegisterLibFunc(&axIsProcessTrusted, ax, "AXIsProcessTrusted")
		purego.RegisterLibFunc(&axIsProcessTrustedWithOptions, ax, "AXIsProcessTrustedWithOptions")
		for fn, name := range map[any]string{
			&cgEventTapCreate: "CGEventTapCreate", &cgEventTapEnable: "CGEventTapEnable",
			&cgEventGetIntegerValueField: "CGEventGetIntegerValueField",
		} {
			purego.RegisterLibFunc(fn, cg, name)
		}
		for fn, name := range map[any]string{
			&cfMachPortCreateRunLoopSource: "CFMachPortCreateRunLoopSource", &cfMachPortInvalidate: "CFMachPortInvalidate",
			&cfRunLoopGetMain: "CFRunLoopGetMain", &cfRunLoopAddSource: "CFRunLoopAddSource",
			&cfRunLoopRemoveSource: "CFRunLoopRemoveSource", &cfRelease: "CFRelease",
			&cfDictionaryCreate: "CFDictionaryCreate",
		} {
			purego.RegisterLibFunc(fn, cf, name)
		}
		commonModes = symbolValue(cf, "kCFRunLoopCommonModes")
		key := symbolValue(ax, "kAXTrustedCheckOptionPrompt")
		yes := symbolValue(cf, "kCFBooleanTrue")
		keyCallbacks, _ := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks")
		valueCallbacks, _ := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks")
		promptOptions = cfDictionaryCreate(0, &key, &yes, 1, keyCallbacks, valueCallbacks)
		callback = purego.NewCallback(tapped)
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

// Trusted says whether MonitorBar has the Accessibility permission. An
// update signed anew loses it, until the user adds the app again.
func Trusted() bool {
	load()
	return axIsProcessTrusted() != 0
}

// RequestAccess asks for the Accessibility permission: macOS adds the app
// to the list and offers to open System Settings.
func RequestAccess() {
	load()
	axIsProcessTrustedWithOptions(promptOptions)
}

// Running says whether the keys are being taken.
func Running() bool { return tap != 0 }

// Start takes the keys for h, on the main thread.
func Start(h Handler) error {
	load()
	handler = h
	if tap != 0 {
		return nil
	}
	mask := uint64(1)<<systemDefined | 1<<keyDown | 1<<keyUp
	t := cgEventTapCreate(sessionTap, headInsert, defaultTap, mask, callback, 0)
	if t == 0 {
		return ErrNotTrusted
	}
	tap, source = t, cfMachPortCreateRunLoopSource(0, t, 0)
	cfRunLoopAddSource(cfRunLoopGetMain(), source, commonModes)
	cgEventTapEnable(tap, true)
	return nil
}

// Stop gives the keys back, on the main thread.
func Stop() {
	if tap == 0 {
		return
	}
	cfRunLoopRemoveSource(cfRunLoopGetMain(), source, commonModes)
	cfMachPortInvalidate(tap)
	cfRelease(source)
	cfRelease(tap)
	tap, source = 0, 0
}

// tapped is the event tap's callback: it returns the event to let it
// through, or nil to take it.
func tapped(proxy uintptr, kind uint32, event, info uintptr) uintptr {
	switch kind {
	case disabledTimeout, disabledByInput: // macOS turned the tap off: on again
		if tap != 0 {
			cgEventTapEnable(tap, true)
		}
		return event
	case keyDown, keyUp:
		// Some keyboards send the brightness keys as plain keys.
		const keycodeField = 9 // kCGKeyboardEventKeycode
		var k Key
		switch cgEventGetIntegerValueField(event, keycodeField) {
		case 144:
			k = BrightnessUp
		case 145:
			k = BrightnessDown
		default:
			return event
		}
		return take(event, k, kind == keyDown)
	case systemDefined:
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
		defer pool.Send(objc.RegisterName("drain"))
		ns := objc.ID(objc.GetClass("NSEvent")).Send(objc.RegisterName("eventWithCGEvent:"), event)
		if ns == 0 || objc.Send[int16](ns, objc.RegisterName("subtype")) != auxControlButton {
			return event
		}
		data := objc.Send[int](ns, objc.RegisterName("data1"))
		var k Key
		switch data >> 16 & 0xFFFF { // NX_KEYTYPE_*
		case 0:
			k = VolumeUp
		case 1:
			k = VolumeDown
		case 2:
			k = BrightnessUp
		case 3:
			k = BrightnessDown
		case 7:
			k = Mute
		default:
			return event
		}
		return take(event, k, data>>8&0xFF == keyStateDown)
	}
	return event
}

func take(event uintptr, k Key, down bool) uintptr {
	if handler != nil && handler(k, down) {
		return 0
	}
	return event
}
