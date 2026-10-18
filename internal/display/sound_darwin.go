//go:build darwin

package display

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type audioPropertyAddress struct{ selector, scope, element uint32 }

var (
	audioObjectGetPropertyData func(object uint32, address *audioPropertyAddress, qualifierSize uint32, qualifier uintptr, size *uint32, data unsafe.Pointer) int32
	audioOnce                  sync.Once
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
	})
	if audioObjectGetPropertyData == nil {
		return false
	}
	const systemObject = 1
	get := func(object uint32, selector string) (uint32, bool) {
		address := audioPropertyAddress{fourCC(selector), fourCC("glob"), 0}
		var value uint32
		size := uint32(unsafe.Sizeof(value))
		return value, audioObjectGetPropertyData(object, &address, 0, 0, &size, unsafe.Pointer(&value)) == 0
	}
	device, ok := get(systemObject, "dOut") // the default output device
	if !ok || device == 0 {
		return false
	}
	transport, ok := get(device, "tran")
	return ok && (transport == fourCC("hdmi") || transport == fourCC("dprt"))
}
