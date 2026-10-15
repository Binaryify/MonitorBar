//go:build darwin

package display

import (
	"sync"

	"github.com/ebitengine/purego"
)

// DisplayServices, a private framework, gives the built-in display's
// brightness, which macOS's automatic brightness sets.
var (
	dsGetBrightness func(id uint32, brightness *float32) int32
	dsOnce          sync.Once
	dsOK            bool
)

// BuiltinBrightness is the brightness of a MacBook's own display, 0...1,
// and false without one.
func BuiltinBrightness() (float64, bool) {
	load()
	dsOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/PrivateFrameworks/DisplayServices.framework/DisplayServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		if _, err := purego.Dlsym(lib, "DisplayServicesGetBrightness"); err != nil {
			return
		}
		purego.RegisterLibFunc(&dsGetBrightness, lib, "DisplayServicesGetBrightness")
		dsOK = true
	})
	if !dsOK {
		return 0, false
	}
	var ids [16]uint32
	var n uint32
	cgGetActiveDisplayList(uint32(len(ids)), &ids[0], &n)
	for _, id := range ids[:n] {
		if cgDisplayIsBuiltin(id) == 0 {
			continue
		}
		var b float32
		if dsGetBrightness(id, &b) != 0 {
			return 0, false
		}
		return float64(b), true
	}
	return 0, false
}
