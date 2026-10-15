//go:build darwin

package display

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
)

// Calls for listing every display, turning one off and on in software, and
// finding the DDC/CI channel of a given display. CGSConfigureDisplayEnabled
// and CoreDisplay_DisplayCreateInfoDictionary are private but long stable:
// BetterDisplay, Lunar and m1ddc rely on them.
var (
	cgGetOnlineDisplayList       func(max uint32, ids *uint32, count *uint32) int32
	cgDisplayVendorNumber        func(id uint32) uint32
	cgDisplayModelNumber         func(id uint32) uint32
	cgDisplayIsActive            func(id uint32) uint32
	cgCancelDisplayConfiguration func(config uintptr) int32
	cgsConfigureDisplayEnabled   func(config uintptr, id uint32, enabled bool) int32
	coreDisplayCreateInfo        func(id uint32) uintptr

	ioRegistryEntryCopyFromPath       func(mainPort uint32, path uintptr) uint32
	ioRegistryEntryGetRegistryEntryID func(entry uint32, id *uint64) int32
	ioRegistryGetRootEntry            func(mainPort uint32) uint32
	ioRegistryEntryCreateIterator     func(entry uint32, plane string, options uint32, iter *uint32) int32
	ioObjectConformsTo                func(obj uint32, class string) uint32
	ioRegistryEntryGetName            func(entry uint32, name *byte) int32

	screensOnce sync.Once
	screensErr  error
)

func loadScreens() error {
	screensOnce.Do(func() {
		load()
		open := func(path string) uintptr {
			h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				screensErr = err
			}
			return h
		}
		cg := open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		io := open("/System/Library/Frameworks/IOKit.framework/IOKit")
		cd := open("/System/Library/Frameworks/CoreDisplay.framework/CoreDisplay")
		if screensErr != nil {
			return
		}
		register := func(lib uintptr, fns map[any]string) {
			for fn, name := range fns {
				if _, err := purego.Dlsym(lib, name); err != nil {
					screensErr = err
					return
				}
				purego.RegisterLibFunc(fn, lib, name)
			}
		}
		register(cg, map[any]string{
			&cgGetOnlineDisplayList: "CGGetOnlineDisplayList", &cgDisplayVendorNumber: "CGDisplayVendorNumber",
			&cgDisplayModelNumber: "CGDisplayModelNumber", &cgDisplayIsActive: "CGDisplayIsActive",
			&cgCancelDisplayConfiguration: "CGCancelDisplayConfiguration",
			&cgsConfigureDisplayEnabled:   "CGSConfigureDisplayEnabled",
		})
		register(cd, map[any]string{&coreDisplayCreateInfo: "CoreDisplay_DisplayCreateInfoDictionary"})
		register(io, map[any]string{
			&ioRegistryEntryCopyFromPath: "IORegistryEntryCopyFromPath", &ioRegistryEntryGetRegistryEntryID: "IORegistryEntryGetRegistryEntryID",
			&ioRegistryGetRootEntry: "IORegistryGetRootEntry", &ioRegistryEntryCreateIterator: "IORegistryEntryCreateIterator",
			&ioObjectConformsTo: "IOObjectConformsTo", &ioRegistryEntryGetName: "IORegistryEntryGetName",
		})
	})
	return screensErr
}

// OnlineScreens lists every display connected, including those turned off
// with SetScreenEnabled. macOS drops a disabled display from this list on
// some systems, so callers keep its ID to turn it back on.
func OnlineScreens() []Screen {
	if loadScreens() != nil {
		return nil
	}
	var ids [16]uint32
	var n uint32
	if cgGetOnlineDisplayList(uint32(len(ids)), &ids[0], &n) != 0 {
		return nil
	}
	main := cgMainDisplayID()
	out := make([]Screen, 0, n)
	for _, id := range ids[:n] {
		out = append(out, Screen{
			ID: id, Vendor: cgDisplayVendorNumber(id), Model: cgDisplayModelNumber(id),
			Builtin: cgDisplayIsBuiltin(id) != 0, Active: cgDisplayIsActive(id) != 0, Main: id == main,
		})
	}
	return out
}

// SetScreenEnabled turns a display on or off in software, for this login
// session: off, it leaves the arrangement as if unplugged, while the cable
// and the monitor's link stay up, so it comes back at once.
func SetScreenEnabled(id uint32, on bool) error {
	if err := loadScreens(); err != nil {
		return err
	}
	var config uintptr
	if cgBeginDisplayConfiguration(&config) != 0 {
		return errors.New("display: cannot begin configuration")
	}
	if cgsConfigureDisplayEnabled(config, id, on) != 0 {
		cgCancelDisplayConfiguration(config)
		return errors.New("display: the system refused to change the display")
	}
	if cgCompleteDisplayConfiguration(config, 1 /* kCGConfigureForSession */) != 0 {
		return errors.New("display: the system refused to change the display")
	}
	return nil
}

// OpenScreenDDC opens the DDC/CI channel of one display, found the way
// m1ddc does: the display's framebuffer in the I/O registry, then the
// external AV service that follows it. It doesn't check that the monitor
// answers: some take writes while reading nothing back.
func OpenScreenDDC(id uint32) (*DDC, error) {
	if err := loadScreens(); err != nil {
		return nil, err
	}
	if err := loadIOAV(); err != nil {
		return nil, err
	}
	service, err := avServiceFor(id)
	if err != nil {
		return nil, err
	}
	return newDDC(&ioavDDC{service: service}), nil
}

func avServiceFor(id uint32) (uintptr, error) {
	info := coreDisplayCreateInfo(id)
	if info == 0 {
		return 0, errors.New("display: no info for the display")
	}
	defer cfRelease(info)
	key := cfStringCreateWithCString(0, "IODisplayLocation", utf8Encoding)
	defer cfRelease(key)
	location := cfDictionaryGetValue(info, key)
	if location == 0 {
		return 0, errors.New("display: no I/O registry location")
	}
	adapter := ioRegistryEntryCopyFromPath(0, location)
	if adapter == 0 {
		return 0, errors.New("display: framebuffer not found")
	}
	var adapterID uint64
	ok := ioRegistryEntryGetRegistryEntryID(adapter, &adapterID) == 0
	ioObjectRelease(adapter)
	if !ok {
		return 0, errors.New("display: framebuffer has no ID")
	}

	var iter uint32
	if ioRegistryEntryCreateIterator(ioRegistryGetRootEntry(0), "IOService", 1 /* recursively */, &iter) != 0 {
		return 0, errors.New("display: cannot walk the I/O registry")
	}
	defer ioObjectRelease(iter)
	locationKey := cfStringCreateWithCString(0, "Location", utf8Encoding)
	defer cfRelease(locationKey)
	matches := false
	name := make([]byte, 128)
	for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
		if ioObjectConformsTo(entry, "IOMobileFramebuffer") != 0 {
			var fb uint64
			matches = ioRegistryEntryGetRegistryEntryID(entry, &fb) == 0 && fb == adapterID
			ioObjectRelease(entry)
			continue
		}
		if !matches || ioRegistryEntryGetName(entry, &name[0]) != 0 || cString(&name[0]) != "DCPAVServiceProxy" {
			ioObjectRelease(entry)
			continue
		}
		loc := ioRegistryEntryCreateCFProperty(entry, locationKey, 0, 0)
		external := goCFString(loc) == "External"
		if loc != 0 {
			cfRelease(loc)
		}
		var service uintptr
		if external {
			service = ioavServiceCreateWithService(0, entry)
		}
		ioObjectRelease(entry)
		if service != 0 {
			return service, nil
		}
	}
	return 0, errors.New("display: no DDC/CI channel for the display")
}
