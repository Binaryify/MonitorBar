//go:build darwin

package display

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type audioPropertyAddress struct{ selector, scope, element uint32 }

var (
	audioObjectGetPropertyData     func(object uint32, address *audioPropertyAddress, qualifierSize uint32, qualifier uintptr, size *uint32, data unsafe.Pointer) int32
	audioObjectGetPropertyDataSize func(uint32, *audioPropertyAddress, uint32, uintptr, *uint32) int32
	audioOnce                      sync.Once
)

// fourCC packs a Core Audio four-character code.
func fourCC(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

// SoundToDisplay says whether the system's sound goes to a display, over
// HDMI or DisplayPort, whose volume macOS can't set.
func SoundToDisplay() bool {
	audioOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&audioObjectGetPropertyData, lib, "AudioObjectGetPropertyData")
		purego.RegisterLibFunc(&audioObjectGetPropertyDataSize, lib, "AudioObjectGetPropertyDataSize")
	})
	if audioObjectGetPropertyData == nil {
		return false
	}
	const systemObject = 1
	get := func(object uint32, selector string) (uint32, bool) {
		address := audioPropertyAddress{fourCC(selector), fourCC("glob"), 0}
		var value uint32
		size := uint32(unsafe.Sizeof(value))
		ok := audioObjectGetPropertyData(object, &address, 0, 0, &size, unsafe.Pointer(&value)) == 0
		return value, ok
	}
	device, ok := get(systemObject, "dOut") // the default output device
	if !ok || device == 0 {
		return false
	}
	return outputContainsDisplay(device, func(id uint32) (string, []uint32) {
		t, ok := get(id, "tran")
		if !ok {
			return "", nil
		}
		if t == fourCC("hdmi") {
			return "hdmi", nil
		}
		if t == fourCC("dprt") {
			return "dprt", nil
		}
		if t != fourCC("grup") {
			return "", nil
		}
		address := audioPropertyAddress{fourCC("agrp"), fourCC("glob"), 0}
		var size uint32
		if audioObjectGetPropertyDataSize(id, &address, 0, 0, &size) != 0 || size == 0 || size%4 != 0 || size > 4096 {
			return "", nil
		}
		ids := make([]uint32, size/4)
		if audioObjectGetPropertyData(id, &address, 0, 0, &size, unsafe.Pointer(&ids[0])) != 0 {
			return "", nil
		}
		return "grup", ids[:size/4]
	})
}
