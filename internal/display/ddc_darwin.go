//go:build darwin

package display

import (
	"bytes"
	"errors"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Apple Silicon reaches the display's I2C bus through these IOKit calls,
// which Apple doesn't document; DDC/CI talks to address 0x37 on it.
var (
	ioavServiceCreateWithService  func(alloc uintptr, service uint32) uintptr
	ioavServiceReadI2C            func(service uintptr, chip, offset uint32, buf *byte, size uint32) int32
	ioavServiceWriteI2C           func(service uintptr, chip, offset uint32, buf *byte, size uint32) int32
	ioRegistryGetRootEntry        func(port uint32) uint32
	ioRegistryEntryCreateIterator func(entry uint32, plane string, options uint32, iter *uint32) int32
	ioRegistryEntryGetName        func(entry uint32, name *byte) int32
	ioavOnce                      sync.Once
	ioavErr                       error
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
			&ioavServiceCreateWithService:  "IOAVServiceCreateWithService",
			&ioavServiceReadI2C:            "IOAVServiceReadI2C",
			&ioavServiceWriteI2C:           "IOAVServiceWriteI2C",
			&ioRegistryGetRootEntry:        "IORegistryGetRootEntry",
			&ioRegistryEntryCreateIterator: "IORegistryEntryCreateIterator",
			&ioRegistryEntryGetName:        "IORegistryEntryGetName",
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

// ddcTransport opens the monitor's DDC/CI channel: the AV service where the
// Mac has one (Apple Silicon), else the framebuffer's own I2C bus (Intel).
func (m Monitor) ddcTransport() (ddcTransport, error) {
	if t, err := m.avDDCTransport(); err == nil {
		return t, nil
	}
	return m.framebufferDDC()
}

// avIdentity is what IOKit tells of the display behind a framebuffer, and
// CoreGraphics of a display, to pair the two.
type avIdentity struct {
	vendor, product, serial uint32
	name                    string
}

// avDDCTransport opens the AV service of the display (Apple Silicon). Each
// external AV service hangs under the framebuffer driving it, after it in
// the registry, as MonitorControl pairs them: the display is told by the
// EDID identity the framebuffer reports.
func (m Monitor) avDDCTransport() (ddcTransport, error) {
	if err := loadIOAV(); err != nil {
		return nil, err
	}
	var iter uint32
	if ioRegistryEntryCreateIterator(ioRegistryGetRootEntry(0), "IOService", 1 /* kIORegistryIterateRecursively */, &iter) != 0 {
		return nil, errors.New("display: no registry")
	}
	defer ioObjectRelease(iter)
	var (
		framebuffer avIdentity
		ids         []avIdentity
		entries     []uint32
	)
	for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
		switch registryName(entry) {
		case "AppleCLCD2", "IOMobileFramebufferShim":
			framebuffer = framebufferIdentity(entry)
		case "DCPAVServiceProxy":
			if stringProperty(entry, "Location") == "External" {
				ids, entries = append(ids, framebuffer), append(entries, entry)
				continue // kept until one is chosen
			}
		}
		ioObjectRelease(entry)
	}
	defer func() {
		for _, e := range entries {
			ioObjectRelease(e)
		}
	}()
	i := pickAV(ids, avIdentity{cgDisplayVendorNumber(m.handle), cgDisplayModelNumber(m.handle), cgDisplaySerialNumber(m.handle), m.Name}, m.order)
	if i < 0 {
		return nil, errors.New("display: no external AV service")
	}
	if service := ioavServiceCreateWithService(0, entries[i]); service != 0 {
		return &ioavDDC{service: service}, nil
	}
	return nil, errors.New("display: the AV service did not open")
}

// pickAV chooses the AV service of the display: the one whose framebuffer
// reports the display best, between identical displays the one in the
// display's place, and by place alone when no framebuffer reports any. The
// first display keeps the only service there is, as before displays were
// paired. -1 for none.
func pickAV(ids []avIdentity, want avIdentity, order int) int {
	if len(ids) == 1 && order == 0 {
		return 0
	}
	if !slices.ContainsFunc(ids, func(id avIdentity) bool { return id.vendor != 0 }) {
		if order < len(ids) {
			return order
		}
		return -1
	}
	score := func(id avIdentity) int {
		n := 0
		for _, same := range []bool{id.vendor == want.vendor, id.product == want.product, id.serial == want.serial && want.serial != 0,
			id.name != "" && id.name == want.name} {
			if same {
				n++
			}
		}
		if id.vendor != want.vendor || id.product != want.product {
			n = 0 // another model: never this display
		}
		return n
	}
	best, tied := -1, []int(nil)
	for i, id := range ids {
		switch s := score(id); {
		case s == 0:
		case best < 0 || s > score(ids[best]):
			best, tied = i, []int{i}
		case s == score(ids[best]):
			tied = append(tied, i)
		}
	}
	if len(tied) > 1 {
		return tied[min(order, len(tied)-1)]
	}
	return best
}

// framebufferIdentity reads DisplayAttributes > ProductAttributes, which a
// framebuffer has while a display is on it.
func framebufferIdentity(entry uint32) avIdentity {
	key := cfStringCreateWithCString(0, "DisplayAttributes", utf8Encoding)
	defer cfRelease(key)
	attributes := ioRegistryEntryCreateCFProperty(entry, key, 0, 0)
	if attributes == 0 {
		return avIdentity{}
	}
	defer cfRelease(attributes)
	product := dictValue(attributes, "ProductAttributes")
	if product == 0 {
		return avIdentity{}
	}
	number := func(name string) uint32 {
		var v int32
		if n := dictValue(product, name); n != 0 {
			cfNumberGetValue(n, cfNumberSInt32Type, unsafe.Pointer(&v))
		}
		return uint32(v)
	}
	return avIdentity{number("LegacyManufacturerID"), number("ProductID"), number("SerialNumber"), goCFString(dictValue(product, "ProductName"))}
}

func dictValue(dict uintptr, name string) uintptr {
	key := cfStringCreateWithCString(0, name, utf8Encoding)
	defer cfRelease(key)
	return cfDictionaryGetValue(dict, key)
}

func stringProperty(entry uint32, name string) string {
	key := cfStringCreateWithCString(0, name, utf8Encoding)
	defer cfRelease(key)
	value := ioRegistryEntryCreateCFProperty(entry, key, 0, 0)
	if value == 0 {
		return ""
	}
	defer cfRelease(value)
	return goCFString(value)
}

func registryName(entry uint32) string {
	var name [128]byte // io_name_t
	if ioRegistryEntryGetName(entry, &name[0]) != 0 {
		return ""
	}
	return cString(&name[0])
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
