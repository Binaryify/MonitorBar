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

// CurrentAudioRoute follows the default output and its active aggregate members.
// Unavailable properties fail closed, leaving unrelated outputs with macOS.
func CurrentAudioRoute() AudioRoute {
	audioOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&audioObjectGetPropertyData, lib, "AudioObjectGetPropertyData")
		purego.RegisterLibFunc(&audioObjectGetPropertyDataSize, lib, "AudioObjectGetPropertyDataSize")
	})
	if audioObjectGetPropertyData == nil {
		return OtherAudio
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
		return OtherAudio
	}
	return classifyAudioRoute(device, func(id uint32) uint32 {
		transport, _ := get(id, "tran")
		return transport
	}, activeAudioMembers)
}

func activeAudioMembers(id uint32) []uint32 {
	address := audioPropertyAddress{fourCC("agrp"), fourCC("glob"), 0}
	var size uint32
	if audioObjectGetPropertyDataSize(id, &address, 0, 0, &size) != 0 || size == 0 || size%4 != 0 || size > 65536 {
		return nil
	}
	members := make([]uint32, size/4)
	if audioObjectGetPropertyData(id, &address, 0, 0, &size, unsafe.Pointer(&members[0])) != 0 || size%4 != 0 || size/4 > uint32(len(members)) {
		return nil
	}
	return members[:size/4]
}
