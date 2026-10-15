//go:build darwin

package display

import (
	"bytes"
	"errors"
	"sync"
	"time"

	"github.com/ebitengine/purego"
)

// Apple Silicon reaches the display's I2C bus through these IOKit calls,
// which Apple doesn't document; DDC/CI talks to address 0x37 on it.
var (
	ioavServiceCreateWithService func(alloc uintptr, service uint32) uintptr
	ioavServiceReadI2C           func(service uintptr, chip, offset uint32, buf *byte, size uint32) int32
	ioavServiceWriteI2C          func(service uintptr, chip, offset uint32, buf *byte, size uint32) int32
	ioavOnce                     sync.Once
	ioavErr                      error
)

func loadIOAV() error {
	ioavOnce.Do(func() {
		load()
		io, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			ioavErr = err
			return
		}
		for fn, name := range map[any]string{
			&ioavServiceCreateWithService: "IOAVServiceCreateWithService",
			&ioavServiceReadI2C:           "IOAVServiceReadI2C",
			&ioavServiceWriteI2C:          "IOAVServiceWriteI2C",
		} {
			if _, err := purego.Dlsym(io, name); err != nil { // Intel Macs lack them
				ioavErr = err
				return
			}
			purego.RegisterLibFunc(fn, io, name)
		}
	})
	return ioavErr
}

type ioavDDC struct{ service uintptr }

// avMatch is one DCPAVServiceProxy, still owned by the caller.
type avMatch struct {
	entry                 uint32
	vendor, model, serial uint32
	external              bool
}

// ddcTransport opens the AV service of this display. The service is matched
// by vendor, model and serial; failing that, by its place among the
// external (or built-in) services, which is how a single external display
// was always reached.
func (m Monitor) ddcTransport() (ddcTransport, error) {
	if err := loadIOAV(); err != nil {
		return nil, err
	}
	matches := avMatches()
	defer func() {
		for _, a := range matches {
			ioObjectRelease(a.entry)
		}
	}()
	entry := pickAV(m, matches)
	if entry == 0 {
		return nil, errors.New("display: no AV service")
	}
	service := ioavServiceCreateWithService(0, entry)
	if service == 0 {
		return nil, errors.New("display: no AV service")
	}
	return &ioavDDC{service: service}, nil
}

func avMatches() []avMatch {
	var iter uint32
	if ioServiceGetMatchingServices(0, ioServiceMatching("DCPAVServiceProxy"), &iter) != 0 {
		return nil
	}
	defer ioObjectRelease(iter)
	key := cfStringCreateWithCString(0, "Location", utf8Encoding)
	defer cfRelease(key)
	var out []avMatch
	for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
		location := ioRegistryEntryCreateCFProperty(entry, key, 0, 0)
		external := goCFString(location) == "External"
		if location != 0 {
			cfRelease(location)
		}
		vendor, model, serial := productIDs(entry)
		out = append(out, avMatch{entry: entry, vendor: vendor, model: model, serial: serial, external: external})
	}
	return out
}

// productIDs reads the EDID identity published on an AV service, or zeros
// when the service doesn't say.
func productIDs(entry uint32) (vendor, model, serial uint32) {
	attrsKey := cfStringCreateWithCString(0, "DisplayAttributes", utf8Encoding)
	defer cfRelease(attrsKey)
	attrs := ioRegistryEntryCreateCFProperty(entry, attrsKey, 0, 0)
	if attrs == 0 {
		return 0, 0, 0
	}
	defer cfRelease(attrs)
	productKey := cfStringCreateWithCString(0, "ProductAttributes", utf8Encoding)
	defer cfRelease(productKey)
	product := cfDictionaryGetValue(attrs, productKey)
	if product == 0 {
		return 0, 0, 0
	}
	num := func(name string) uint32 {
		k := cfStringCreateWithCString(0, name, utf8Encoding)
		defer cfRelease(k)
		return cfNum(cfDictionaryGetValue(product, k))
	}
	vendor = num("LegacyManufacturerID")
	if vendor == 0 {
		vendor = num("VendorID")
	}
	return vendor, num("ProductID"), num("SerialNumber")
}

func cfNum(ref uintptr) uint32 {
	if ref == 0 || cfGetTypeID(ref) != cfNumberGetTypeID() {
		return 0
	}
	var v int64
	if !cfNumberGetValue(ref, 4 /* kCFNumberSInt64Type */, &v) {
		return 0
	}
	return uint32(v)
}

// pickAV chooses the service for m. Identical panels share an identity, so
// twin is the index among that identity; when the service doesn't publish
// one, slot is the index among external or built-in services.
func pickAV(m Monitor, matches []avMatch) uint32 {
	wantExternal := !m.Builtin
	if m.vendor != 0 || m.model != 0 || m.serial != 0 {
		var same []avMatch
		for _, a := range matches {
			if a.external == wantExternal && a.vendor == m.vendor && a.model == m.model && a.serial == m.serial {
				same = append(same, a)
			}
		}
		if m.twin >= 0 && m.twin < len(same) {
			return same[m.twin].entry
		}
	}
	n := 0
	for _, a := range matches {
		if a.external != wantExternal {
			continue
		}
		if n == m.slot {
			return a.entry
		}
		n++
	}
	return 0
}

func (d *ioavDDC) read(code byte) (value, maximum int, ok bool) {
	for range 3 {
		reply := make([]byte, 11)
		if d.send([]byte{0x01, code}) && d.receive(reply) && reply[2] == 0x02 && reply[4] == code {
			if reply[3] != 0 { // the monitor answered "unsupported code"
				return 0, 0, false
			}
			return int(reply[8])<<8 | int(reply[9]), int(reply[6])<<8 | int(reply[7]), true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0, 0, false
}

func (d *ioavDDC) write(code byte, value int) bool {
	return d.send([]byte{0x03, code, byte(value >> 8), byte(value)})
}

func (d *ioavDDC) capabilities() string {
	var caps []byte
	for len(caps) < 4096 {
		reply := make([]byte, 38)
		offset := len(caps)
		if !d.send([]byte{0xF3, byte(offset >> 8), byte(offset)}) || !d.receive(reply) || reply[2] != 0xE3 {
			break
		}
		end := 2 + int(reply[1]&0x7F)
		if end < 5 {
			break
		}
		chunk := reply[5:end]
		caps = append(caps, chunk...)
		if len(chunk) == 0 || bytes.IndexByte(chunk, 0) >= 0 {
			break
		}
	}
	if i := bytes.IndexByte(caps, 0); i >= 0 {
		caps = caps[:i]
	}
	return string(caps)
}

func (d *ioavDDC) close() { cfRelease(d.service) }

// send writes a DDC/CI packet: length, payload, checksum. It goes out twice,
// as some monitors drop the first one after idling.
func (d *ioavDDC) send(payload []byte) bool {
	packet := append([]byte{0x80 | byte(len(payload))}, payload...)
	sum := byte(0x6E ^ 0x51)
	for _, b := range packet {
		sum ^= b
	}
	packet = append(packet, sum)
	sent := false
	for range 2 {
		time.Sleep(10 * time.Millisecond)
		if ioavServiceWriteI2C(d.service, 0x37, 0x51, &packet[0], uint32(len(packet))) == 0 {
			sent = true
		}
	}
	return sent
}

// receive reads a reply and checks its length and checksum.
func (d *ioavDDC) receive(reply []byte) bool {
	time.Sleep(50 * time.Millisecond)
	if ioavServiceReadI2C(d.service, 0x37, 0x51, &reply[0], uint32(len(reply))) != 0 {
		return false
	}
	length := int(reply[1] & 0x7F)
	if length < 1 || length+2 >= len(reply) {
		return false
	}
	sum := byte(0x50)
	for _, b := range reply[:2+length] {
		sum ^= b
	}
	return sum == reply[2+length]
}
