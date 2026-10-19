package display

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// MCCS VCP codes MonitorBar uses over DDC/CI.
const (
	VCPBrightness byte = 0x10
	VCPContrast   byte = 0x12
	VCPRedGain    byte = 0x16
	VCPGreenGain  byte = 0x18
	VCPBlueGain   byte = 0x1A
	VCPInput      byte = 0x60
	VCPVolume     byte = 0x62
	VCPSharpness  byte = 0x87
)

// ddcTransport is what each platform implements: blocking DDC/CI reads and
// writes, tens of milliseconds each. ddc_darwin.go speaks I2C through
// IOAVService (Apple Silicon), ddc_framebuffer_darwin.go through the
// framebuffer's I2C bus (Intel), ddc_windows.go the Monitor Configuration
// API (dxva2).
type ddcTransport interface {
	read(code byte) (value, maximum int, ok bool)
	write(code byte, value int) bool
	capabilities() string
	close()
}

// DDC controls the monitor's own settings over the video cable. Get blocks;
// Set returns at once and sends only the latest value of each code, so a
// dragged slider doesn't queue up slow writes.
type DDC struct {
	t       ddcTransport
	mu      sync.Mutex
	pending map[byte]int
	wake    chan struct{}
	done    chan struct{}
}

// OpenDDC returns the monitor's DDC/CI channel, or an error when the
// monitor doesn't answer it: many don't, or have it off in their menu.
func (m Monitor) OpenDDC() (*DDC, error) {
	t, err := m.ddcTransport()
	if err != nil {
		return nil, err
	}
	if _, _, ok := t.read(VCPBrightness); !ok {
		t.close()
		return nil, fmt.Errorf("display: no DDC/CI answer")
	}
	d := &DDC{t: t, pending: map[byte]int{}, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go d.writer()
	return d, nil
}

// Get reads a VCP code: its value, its maximum, and whether the monitor
// supports it.
func (d *DDC) Get(code byte) (value, maximum int, ok bool) {
	d.mu.Lock() // one transaction on the bus at a time
	defer d.mu.Unlock()
	return d.t.read(code)
}

// Set queues a write of a VCP code.
func (d *DDC) Set(code byte, value int) {
	d.mu.Lock()
	d.pending[code] = value
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Capabilities is the monitor's MCCS capability string, "" if unreadable.
func (d *DDC) Capabilities() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.t.capabilities()
}

// Close stops the writer and releases the monitor.
func (d *DDC) Close() {
	close(d.done)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.t.close()
}

func (d *DDC) writer() {
	for {
		select {
		case <-d.done:
			return
		case <-d.wake:
		}
		for {
			d.mu.Lock()
			var code byte
			var value int
			found := false
			for code, value = range d.pending {
				found = true
				break
			}
			if !found {
				d.mu.Unlock()
				break
			}
			delete(d.pending, code)
			d.t.write(code, value)
			d.mu.Unlock()
		}
	}
}

// inputOverrides names the inputs of monitor models that report them
// differently from MCCS, keyed by the model's name as the system gives it.
var inputOverrides = map[string]map[int]string{
	"Redmi 27 NU": {0x10: "USB-C"}, // its USB-C reports as MCCS's second DP (issue #2)
}

// InputNameFor names an input of a monitor model, with its overrides.
func InputNameFor(model string, code int) string {
	for name, overrides := range inputOverrides {
		if strings.EqualFold(strings.TrimSpace(model), name) {
			if n, ok := overrides[code]; ok {
				return n
			}
		}
	}
	return InputName(code)
}

// InputName names an MCCS input source code.
func InputName(code int) string {
	names := map[int]string{0x01: "VGA", 0x03: "DVI", 0x04: "DVI 2", 0x0F: "DP", 0x10: "DP 2",
		0x11: "HDMI 1", 0x12: "HDMI 2", 0x13: "HDMI 3", 0x19: "USB-C", 0x1B: "USB-C", 0x1C: "USB-C 2"}
	if n, ok := names[code]; ok {
		return n
	}
	return fmt.Sprintf("%02Xh", code) // the code, as MCCS writes it
}

// ParseCapabilities reads the vcp(...) section of an MCCS capability
// string, e.g. "vcp(10 12 60(0F 11 12))", into each code and its values.
func ParseCapabilities(caps string) map[byte][]int {
	start := strings.Index(strings.ToLower(caps), "vcp(")
	if start < 0 {
		return nil
	}
	out := map[byte][]int{}
	depth, token := 1, ""
	var current byte
	haveCurrent := false
	flush := func() {
		// Some monitors leave out the spaces: "101260" is 10 12 60.
		for i := 0; i+2 <= len(token); i += 2 {
			b, err := strconv.ParseUint(token[i:i+2], 16, 8)
			if err != nil {
				continue
			}
			if depth == 1 {
				current, haveCurrent = byte(b), true
				out[current] = out[current][:0:0]
			} else if haveCurrent {
				out[current] = append(out[current], int(b))
			}
		}
		token = ""
	}
	for _, r := range caps[start+4:] {
		switch r {
		case '(':
			flush()
			depth++
		case ')':
			flush()
			if depth--; depth == 0 {
				return out
			}
		case ' ':
			flush()
		default:
			token += string(r)
		}
	}
	return out
}
