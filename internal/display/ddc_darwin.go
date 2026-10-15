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

// ddcTransport opens the AV service of the first external display: the app
// manages one external monitor.
func (m Monitor) ddcTransport() (ddcTransport, error) {
	if err := loadIOAV(); err != nil {
		return nil, err
	}
	var iter uint32
	if ioServiceGetMatchingServices(0, ioServiceMatching("DCPAVServiceProxy"), &iter) != 0 {
		return nil, errors.New("display: no AV services")
	}
	defer ioObjectRelease(iter)
	key := cfStringCreateWithCString(0, "Location", utf8Encoding)
	defer cfRelease(key)
	for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
		location := ioRegistryEntryCreateCFProperty(entry, key, 0, 0)
		external := goCFString(location) == "External"
		if location != 0 {
			cfRelease(location)
		}
		var service uintptr
		if external {
			service = ioavServiceCreateWithService(0, entry)
		}
		ioObjectRelease(entry)
		if service != 0 {
			return &ioavDDC{service: service}, nil
		}
	}
	return nil, errors.New("display: no external AV service")
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
