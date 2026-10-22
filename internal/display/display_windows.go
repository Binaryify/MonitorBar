//go:build windows

package display

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SettingsURL opens the Display page of Windows Settings.
const SettingsURL = "ms-settings:display"

type (
	handle     = string  // GDI device name, e.g. \\.\DISPLAY1
	nativeMode = devMode // the DEVMODE EnumDisplaySettings returned
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	powrprof = windows.NewLazySystemDLL("powrprof.dll")

	procEnumDisplayDevices          = user32.NewProc("EnumDisplayDevicesW")
	procEnumDisplaySettings         = user32.NewProc("EnumDisplaySettingsW")
	procChangeDisplaySettingsEx     = user32.NewProc("ChangeDisplaySettingsExW")
	procGetLastInputInfo            = user32.NewProc("GetLastInputInfo")
	procPostMessage                 = user32.NewProc("PostMessageW")
	procGetDisplayConfigBufferSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = user32.NewProc("QueryDisplayConfig")
	procDisplayConfigGetDeviceInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")
	procCreateDC                    = gdi32.NewProc("CreateDCW")
	procDeleteDC                    = gdi32.NewProc("DeleteDC")
	procSetDeviceGammaRamp          = gdi32.NewProc("SetDeviceGammaRamp")
	procGetTickCount                = kernel32.NewProc("GetTickCount")
	procPowerGetActiveScheme        = powrprof.NewProc("PowerGetActiveScheme")
	procPowerReadACValueIndex       = powrprof.NewProc("PowerReadACValueIndex")
)

// devMode is DEVMODEW, display fields of its unions.
type devMode struct {
	DeviceName         [32]uint16
	SpecVersion        uint16
	DriverVersion      uint16
	Size               uint16
	DriverExtra        uint16
	Fields             uint32
	PositionX          int32
	PositionY          int32
	DisplayOrientation uint32
	DisplayFixedOutput uint32
	Color              int16
	Duplex             int16
	YResolution        int16
	TTOption           int16
	Collate            int16
	FormName           [32]uint16
	LogPixels          uint16
	BitsPerPel         uint32
	PelsWidth          uint32
	PelsHeight         uint32
	DisplayFlags       uint32
	DisplayFrequency   uint32
	ICMMethod          uint32
	ICMIntent          uint32
	MediaType          uint32
	DitherType         uint32
	Reserved1          uint32
	Reserved2          uint32
	PanningWidth       uint32
	PanningHeight      uint32
}

type displayDevice struct {
	Size         uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

// DisplayConfig structures (wingdi.h).
type (
	luid struct {
		Low  uint32
		High int32
	}
	pathInfo struct {
		SourceAdapter luid
		SourceID      uint32
		_             [2]uint32 // modeInfoIdx, statusFlags
		TargetAdapter luid
		TargetID      uint32
		_             [9]uint32 // modeInfoIdx ... statusFlags
		Flags         uint32
	}
	deviceInfoHeader struct {
		Type    uint32
		Size    uint32
		Adapter luid
		ID      uint32
	}
	sourceDeviceName struct {
		Header        deviceInfoHeader
		GDIDeviceName [32]uint16
	}
	targetPreferredMode struct {
		Header        deviceInfoHeader
		Width, Height uint32
		_             [52]byte // the signal info, 8-aligned
	}
	targetDeviceName struct {
		Header            deviceInfoHeader
		Flags             uint32
		OutputTechnology  uint32
		ManufactureID     uint16
		ProductCodeID     uint16
		ConnectorInstance uint32
		FriendlyName      [64]uint16
		DevicePath        [128]uint16
	}
)

const (
	enumCurrentSettings  = 0xFFFFFFFF
	attachedToDesktop    = 0x1
	primaryDevice        = 0x4
	dmInterlaced         = 0x2
	qdcOnlyActivePaths   = 0x2
	getSourceName        = 1
	getTargetName        = 2
	getPreferredMode     = 3
	cdsUpdateRegistry    = 0x1
	dispChangeSuccessful = 0
	dmFieldsForMode      = 0x40000 | 0x80000 | 0x100000 | 0x400000 // bits per pixel, width, height, frequency
)

// Target returns the external display, the primary one if several are; a
// laptop's own panel only when no external one is on, as on macOS.
func Target() Monitor {
	var best Monitor
	bestRank := -1
	for i := uint32(0); ; i++ {
		var dd displayDevice
		dd.Size = uint32(unsafe.Sizeof(dd))
		if r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0); r == 0 {
			break
		}
		if dd.StateFlags&attachedToDesktop == 0 {
			continue
		}
		m := Monitor{handle: windows.UTF16ToString(dd.DeviceName[:])}
		m.Name, m.Connection, m.NativeWidth, m.NativeHeight, m.Builtin = targetInfo(m.handle)
		if m.Name == "" {
			m.Name = monitorString(dd.DeviceName[:])
		}
		rank := 0
		if !m.Builtin {
			rank += 2
		}
		if dd.StateFlags&primaryDevice != 0 {
			rank++
		}
		if rank > bestRank {
			best, bestRank = m, rank
		}
	}
	return best
}

// targetInfo finds the monitor's friendly name, cable type, preferred
// (native) resolution and whether it is a laptop's own panel through the
// display path whose source is the GDI device. DSR and similar list modes
// larger than the panel; this one is not.
func targetInfo(device string) (name, connection string, width, height int, internal bool) {
	var nPaths, nModes uint32
	if r, _, _ := procGetDisplayConfigBufferSizes.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&nPaths)), uintptr(unsafe.Pointer(&nModes))); r != 0 || nPaths == 0 {
		return "", "", 0, 0, false
	}
	paths := make([]pathInfo, nPaths)
	modes := make([][64]byte, max(nModes, 1))
	if r, _, _ := procQueryDisplayConfig.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&nPaths)), uintptr(unsafe.Pointer(&paths[0])),
		uintptr(unsafe.Pointer(&nModes)), uintptr(unsafe.Pointer(&modes[0])), 0); r != 0 {
		return "", "", 0, 0, false
	}
	for _, p := range paths[:nPaths] {
		source := sourceDeviceName{Header: deviceInfoHeader{Type: getSourceName, Adapter: p.SourceAdapter, ID: p.SourceID}}
		source.Header.Size = uint32(unsafe.Sizeof(source))
		if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&source))); r != 0 ||
			windows.UTF16ToString(source.GDIDeviceName[:]) != device {
			continue
		}
		target := targetDeviceName{Header: deviceInfoHeader{Type: getTargetName, Adapter: p.TargetAdapter, ID: p.TargetID}}
		target.Header.Size = uint32(unsafe.Sizeof(target))
		if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&target))); r != 0 {
			continue
		}
		preferred := targetPreferredMode{Header: deviceInfoHeader{Type: getPreferredMode, Adapter: p.TargetAdapter, ID: p.TargetID}}
		preferred.Header.Size = uint32(unsafe.Sizeof(preferred))
		procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&preferred)))
		return windows.UTF16ToString(target.FriendlyName[:]), outputTechnology(target.OutputTechnology), int(preferred.Width), int(preferred.Height),
			builtinOutput(target.OutputTechnology)
	}
	return "", "", 0, 0, false
}

func outputTechnology(t uint32) string {
	switch t {
	case 0:
		return "VGA"
	case 4:
		return "DVI"
	case 5:
		return "HDMI"
	case 10:
		return "DisplayPort"
	}
	return ""
}

// builtinOutput tells a laptop's own panel: LVDS, embedded DisplayPort or
// UDI, or the adapter's internal connection.
func builtinOutput(t uint32) bool {
	return t == 6 || t == 11 || t == 13 || t == 0x80000000
}

// monitorString is the monitor's device string, such as "Generic PnP Monitor".
func monitorString(adapter []uint16) string {
	var dd displayDevice
	dd.Size = uint32(unsafe.Sizeof(dd))
	if r, _, _ := procEnumDisplayDevices.Call(uintptr(unsafe.Pointer(&adapter[0])), 0, uintptr(unsafe.Pointer(&dd)), 0); r == 0 {
		return ""
	}
	return windows.UTF16ToString(dd.DeviceString[:])
}

func (m Monitor) device() *uint16 {
	p, _ := syscall.UTF16PtrFromString(m.handle)
	return p
}

func (m Monitor) Modes() []Mode {
	var modes []Mode
	for i := uint32(0); ; i++ {
		var dm devMode
		dm.Size = uint16(unsafe.Sizeof(dm))
		if r, _, _ := procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(m.device())), uintptr(i), uintptr(unsafe.Pointer(&dm))); r == 0 {
			break
		}
		if dm.BitsPerPel >= 24 && dm.DisplayFlags&dmInterlaced == 0 {
			modes = append(modes, newMode(dm))
		}
	}
	return dedupe(modes)
}

func (m Monitor) CurrentMode() (Mode, bool) {
	var dm devMode
	dm.Size = uint16(unsafe.Sizeof(dm))
	r, _, _ := procEnumDisplaySettings.Call(uintptr(unsafe.Pointer(m.device())), enumCurrentSettings, uintptr(unsafe.Pointer(&dm)))
	return newMode(dm), r != 0
}

func newMode(dm devMode) Mode {
	w, h := int(dm.PelsWidth), int(dm.PelsHeight)
	return Mode{Width: w, Height: h, PixelWidth: w, PixelHeight: h, RefreshRate: int(dm.DisplayFrequency), native: dm}
}

func (m Monitor) Apply(mode Mode) error {
	dm := mode.native
	dm.Fields = dmFieldsForMode
	r, _, _ := procChangeDisplaySettingsEx.Call(uintptr(unsafe.Pointer(m.device())), uintptr(unsafe.Pointer(&dm)), 0, cdsUpdateRegistry, 0)
	if int32(r) != dispChangeSuccessful {
		return errors.New("display: the display refused the mode")
	}
	return nil
}

type gammaRamp [3][256]uint16

// identity is the ramp Windows starts with. Changes are made from it, not
// from the current ramp, which a dimmed session may have left behind:
// Windows keeps a ramp after the app that set it quits.
var identity = func() (r gammaRamp) {
	for c := range r {
		for i := range r[c] {
			r[c][i] = uint16(i * 257)
		}
	}
	return r
}()

var gammaDevice string // the display last dimmed, for RestoreGamma

// SetGamma dims and warms the picture through the gamma ramp.
func (m Monitor) SetGamma(brightness, warmth float64) error {
	dc := m.dc()
	if dc == 0 {
		return errors.New("display: no device context")
	}
	defer procDeleteDC.Call(dc)
	gammaDevice = m.handle
	gr, gg, gb := gains(brightness, warmth)
	if neutral(brightness, warmth) {
		gr, gg, gb = 1, 1, 1
	}
	// Windows refuses ramps far from the identity: raise the gains together,
	// keeping their balance (the warmth), until it takes one.
	for floor := 0.0; floor <= 1.0001; floor += 0.05 {
		k := 1.0
		if darkest := min(gr, gg, gb); darkest < floor {
			k = floor / darkest
		}
		var ramp gammaRamp
		for c, gain := range []float64{min(gr*k, 1), min(gg*k, 1), min(gb*k, 1)} {
			for i := range ramp[c] {
				ramp[c][i] = uint16(float64(identity[c][i]) * gain)
			}
		}
		if r, _, _ := procSetDeviceGammaRamp.Call(dc, uintptr(unsafe.Pointer(&ramp))); r != 0 {
			if floor > 0 {
				return ErrGammaLimited
			}
			return nil
		}
	}
	return errors.New("display: Windows refused the gamma ramp")
}

// RestoreGamma puts the identity ramp back; Windows keeps a changed ramp
// after the app quits.
func RestoreGamma() {
	if gammaDevice == "" {
		return
	}
	m := Monitor{handle: gammaDevice}
	if dc := m.dc(); dc != 0 {
		ramp := identity
		procSetDeviceGammaRamp.Call(dc, uintptr(unsafe.Pointer(&ramp)))
		procDeleteDC.Call(dc)
	}
}

// ResetGammaBase does nothing on Windows: changes start from the identity.
func ResetGammaBase() {}

func (m Monitor) dc() uintptr {
	driver, _ := syscall.UTF16PtrFromString("DISPLAY")
	dc, _, _ := procCreateDC.Call(uintptr(unsafe.Pointer(driver)), uintptr(unsafe.Pointer(m.device())), 0, 0)
	return dc
}

// Asleep guesses whether Windows turned the display off: once input has
// been idle longer than the power plan's "turn off the display" timeout.
func (m Monitor) Asleep() bool {
	timeout := displayTimeout()
	return timeout > 0 && IdleSeconds() >= float64(timeout)
}

var (
	videoSubgroup = windows.GUID{Data1: 0x7516b95f, Data2: 0xf776, Data3: 0x4464, Data4: [8]byte{0x8c, 0x53, 0x06, 0x16, 0x7f, 0x40, 0xcc, 0x99}}
	videoIdle     = windows.GUID{Data1: 0x3c0bc021, Data2: 0xc8a8, Data3: 0x4e07, Data4: [8]byte{0xa9, 0x73, 0x6b, 0x14, 0xcb, 0xcb, 0x2b, 0x7e}}
)

// displayTimeout is the active power plan's display timeout in seconds
// (plugged in), 0 for never.
func displayTimeout() uint32 {
	var scheme *windows.GUID
	if r, _, _ := procPowerGetActiveScheme.Call(0, uintptr(unsafe.Pointer(&scheme))); r != 0 || scheme == nil {
		return 0
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(scheme))))
	var seconds uint32
	procPowerReadACValueIndex.Call(0, uintptr(unsafe.Pointer(scheme)), uintptr(unsafe.Pointer(&videoSubgroup)),
		uintptr(unsafe.Pointer(&videoIdle)), uintptr(unsafe.Pointer(&seconds)))
	return seconds
}

// IdleSeconds is the time since the last keyboard or mouse input.
func IdleSeconds() float64 {
	info := struct{ Size, Time uint32 }{Size: 8}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	now, _, _ := procGetTickCount.Call()
	return float64(uint32(now)-info.Time) / 1000
}

// SleepNow turns the displays off until the next input.
func SleepNow() error {
	const hwndBroadcast, wmSysCommand, scMonitorPower, monitorOff = 0xFFFF, 0x0112, 0xF170, 2
	if r, _, err := procPostMessage.Call(hwndBroadcast, wmSysCommand, scMonitorPower, monitorOff); r == 0 {
		return err
	}
	return nil
}
