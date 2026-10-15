//go:build windows

package display

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows speaks DDC/CI itself, through the Monitor Configuration API.
var (
	dxva2 = windows.NewLazySystemDLL("dxva2.dll")

	procEnumDisplayMonitors                     = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo                          = user32.NewProc("GetMonitorInfoW")
	procGetNumberOfPhysicalMonitorsFromHMONITOR = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitorsFromHMONITOR         = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procDestroyPhysicalMonitors                 = dxva2.NewProc("DestroyPhysicalMonitors")
	procGetVCPFeatureAndVCPFeatureReply         = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	procSetVCPFeature                           = dxva2.NewProc("SetVCPFeature")
	procGetCapabilitiesStringLength             = dxva2.NewProc("GetCapabilitiesStringLength")
	procCapabilitiesRequestAndCapabilitiesReply = dxva2.NewProc("CapabilitiesRequestAndCapabilitiesReply")
)

type (
	monitorInfoEx struct {
		Size    uint32
		Monitor windows.Rect
		Work    windows.Rect
		Flags   uint32
		Device  [32]uint16
	}
	physicalMonitor struct {
		Handle      windows.Handle
		Description [128]uint16
	}
)

type dxvaDDC struct{ monitors []physicalMonitor }

// enumCallback is made once (Windows callbacks are a scarce resource) and
// collects the monitors into enumFound, under enumMu.
var (
	enumMu       sync.Mutex
	enumFound    []uintptr
	enumCallback = syscall.NewCallback(func(hmon, hdc, rect, data uintptr) uintptr {
		enumFound = append(enumFound, hmon)
		return 1
	})
)

func displayMonitors() []uintptr {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFound = nil
	procEnumDisplayMonitors.Call(0, 0, enumCallback, 0)
	return enumFound
}

// ddcTransport opens the physical monitors behind the GDI device.
func (m Monitor) ddcTransport() (ddcTransport, error) {
	for _, hmon := range displayMonitors() {
		info := monitorInfoEx{Size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		if r, _, _ := procGetMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&info))); r == 0 ||
			windows.UTF16ToString(info.Device[:]) != m.handle {
			continue
		}
		var n uint32
		if r, _, _ := procGetNumberOfPhysicalMonitorsFromHMONITOR.Call(hmon, uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
			break
		}
		monitors := make([]physicalMonitor, n)
		if r, _, _ := procGetPhysicalMonitorsFromHMONITOR.Call(hmon, uintptr(n), uintptr(unsafe.Pointer(&monitors[0]))); r == 0 {
			break
		}
		return &dxvaDDC{monitors: monitors}, nil
	}
	return nil, errors.New("display: no physical monitor")
}

func (d *dxvaDDC) read(code byte) (value, maximum int, ok bool) {
	var typ, cur, max uint32
	r, _, _ := procGetVCPFeatureAndVCPFeatureReply.Call(uintptr(d.monitors[0].Handle), uintptr(code),
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&max)))
	return int(cur), int(max), r != 0
}

func (d *dxvaDDC) write(code byte, value int) bool {
	r, _, _ := procSetVCPFeature.Call(uintptr(d.monitors[0].Handle), uintptr(code), uintptr(value))
	return r != 0
}

func (d *dxvaDDC) capabilities() string {
	var n uint32
	if r, _, _ := procGetCapabilitiesStringLength.Call(uintptr(d.monitors[0].Handle), uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
		return ""
	}
	buf := make([]byte, n)
	if r, _, _ := procCapabilitiesRequestAndCapabilitiesReply.Call(uintptr(d.monitors[0].Handle), uintptr(unsafe.Pointer(&buf[0])), uintptr(n)); r == 0 {
		return ""
	}
	return windows.ByteSliceToString(buf)
}

func (d *dxvaDDC) close() {
	procDestroyPhysicalMonitors.Call(uintptr(len(d.monitors)), uintptr(unsafe.Pointer(&d.monitors[0])))
}
