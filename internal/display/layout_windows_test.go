//go:build windows

package display

import (
	"testing"
	"unsafe"
)

// The Win32 structures must match their C sizes byte for byte.
func TestStructSizes(t *testing.T) {
	for name, got := range map[string]uintptr{
		"DEVMODEW": unsafe.Sizeof(devMode{}), "DISPLAY_DEVICEW": unsafe.Sizeof(displayDevice{}),
		"DISPLAYCONFIG_PATH_INFO": unsafe.Sizeof(pathInfo{}), "DISPLAYCONFIG_SOURCE_DEVICE_NAME": unsafe.Sizeof(sourceDeviceName{}),
		"DISPLAYCONFIG_TARGET_DEVICE_NAME": unsafe.Sizeof(targetDeviceName{}),
	} {
		want := map[string]uintptr{"DEVMODEW": 220, "DISPLAY_DEVICEW": 840, "DISPLAYCONFIG_PATH_INFO": 72,
			"DISPLAYCONFIG_SOURCE_DEVICE_NAME": 84, "DISPLAYCONFIG_TARGET_DEVICE_NAME": 420}[name]
		if got != want {
			t.Errorf("%s is %d bytes, want %d", name, got, want)
		}
	}
}
