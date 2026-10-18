//go:build darwin

package display

import (
	"bytes"
	"errors"
	"time"
	"unsafe"
)

// Apple Silicon reaches the display's I2C bus through IOAVService; Intel Macs
// (and hackintoshes) don't have the DCP the AV service belongs to, and reach it
// the older way instead: the IOFramebuffer's own I2C bus, driven with
// IOI2CRequest. Both speak the same DDC/CI packets on address 0x37.

// IOI2CRequest as IOKit declares it (packed to 4, 124 bytes). It is kept as
// words so the layout doesn't depend on Go's field alignment.
type i2cRequest struct{ words [31]uint32 }

const (
	i2cNoTransaction     = 0
	i2cSimpleTransaction = 1
	i2cDDCReply          = 2
	i2cReplyDelay        = 50 * time.Millisecond
	i2cSettleDelay       = 10 * time.Millisecond
)

// newI2CRequest fills the fields the bus reads: the two transaction types, the
// addresses (0x6E to write, 0x6F to read, sub-address 0x51), the buffers and
// their sizes.
func newI2CRequest(sendType, replyType uint32, send, reply []byte) *i2cRequest {
	r := &i2cRequest{}
	r.words[0] = sendType
	r.words[1] = replyType
	r.words[2] = 0x6E // send address: the monitor's DDC/CI bus
	r.words[3] = 0x6F // reply address
	r.words[4] = 0x51 << 8 // reply sub-address
	r.words[5] = uint32(i2cReplyDelay / time.Nanosecond)
	r.words[10] = uint32(len(send))
	r.words[14] = uint32(len(reply))
	if len(send) > 0 {
		lo, hi := splitPointer(uintptr(unsafe.Pointer(&send[0])))
		r.words[17], r.words[18] = lo, hi
	}
	if len(reply) > 0 {
		lo, hi := splitPointer(uintptr(unsafe.Pointer(&reply[0])))
		r.words[19], r.words[20] = lo, hi
	}
	return r
}

// result is the transaction's IOReturn: 0 when the bus did what it was asked.
func (r *i2cRequest) result() int32 { return int32(r.words[7]) }

func splitPointer(p uintptr) (lo, hi uint32) { return uint32(p), uint32(uint64(p) >> 32) }

type fbDDC struct {
	connect   uintptr // IOI2CConnectRef
	replyType uint32 // kIOI2CDDCciReplyTransactionType, or simple where unsupported
}

func (d *fbDDC) close() {
	if d.connect != 0 {
		ioI2CInterfaceClose(d.connect, 0)
		d.connect = 0
	}
}

// packet frames a DDC/CI payload: length with its high bit set, the payload,
// the checksum.
func packet(payload []byte) []byte {
	out := append([]byte{0x80 | byte(len(payload))}, payload...)
	sum := byte(0x6E ^ 0x51)
	for _, b := range out {
		sum ^= b
	}
	return append(out, sum)
}

// transact sends one DDC/CI packet and reads the reply, on the same I2C
// transaction. Some monitors drop the first one after idling, so it retries.
func (d *fbDDC) transact(payload, reply []byte) bool {
	buf := append([]byte{0x51}, packet(payload)...) // 0x51: the DDC/CI sub-address
	for range 3 {
		time.Sleep(i2cSettleDelay) // the monitor needs a gap between transactions
		req := newI2CRequest(i2cSimpleTransaction, d.replyType, buf, reply)
		if ioI2CSendRequest(d.connect, 0, req) == 0 && req.result() == 0 {
			return true
		}
	}
	return false
}

// write sends a packet and waits for the monitor to swallow it.
func (d *fbDDC) write(code byte, value int) bool {
	req := newI2CRequest(i2cSimpleTransaction, i2cNoTransaction,
		append([]byte{0x51}, packet([]byte{0x03, code, byte(value >> 8), byte(value)})...), nil)
	if ioI2CSendRequest(d.connect, 0, req) != 0 || req.result() != 0 {
		return false
	}
	time.Sleep(20 * time.Millisecond) // the monitor can't take the next packet before this
	return true
}

func (d *fbDDC) read(code byte) (value, maximum int, ok bool) {
	for range 3 {
		reply := make([]byte, 11)
		if d.transact([]byte{0x01, code}, reply) && validReply(reply, code) {
			if reply[3] != 0 { // the monitor answered "unsupported code"
				return 0, 0, false
			}
			return int(reply[8])<<8 | int(reply[9]), int(reply[6])<<8 | int(reply[7]), true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0, 0, false
}

func (d *fbDDC) capabilities() string {
	var caps []byte
	for len(caps) < 4096 {
		offset := len(caps)
		var reply []byte
		for range 3 { // the first read after opening the bus sometimes comes back short
			reply = make([]byte, 38)
			if d.transact([]byte{0xF3, byte(offset >> 8), byte(offset)}, reply) && reply[2] == 0xE3 {
				break
			}
			reply = nil
			time.Sleep(i2cSettleDelay)
		}
		if reply == nil {
			break
		}
		end := 2 + int(reply[1]&0x7F)
		if end < 5 || end > len(reply) {
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

// validReply checks a DDC/CI reply's address, opcode, VCP code and checksum.
func validReply(reply []byte, code byte) bool {
	if reply[0] != 0x6E || reply[2] != 0x02 || reply[4] != code {
		return false
	}
	sum := byte(0x6F ^ 0x51)
	for _, b := range reply[1:10] { // the checksum covers the sub-address and the payload, not the address byte
		sum ^= b
	}
	return sum == reply[10]
}

// framebufferDDC opens the I2C bus of the IOFramebuffer this display hangs off:
// the Intel path. It finds the framebuffer by the EDID vend/prod/serial triple,
// since a Mac can have several.
func (m Monitor) framebufferDDC() (ddcTransport, error) {
	load()
	var iter uint32
	if ioServiceGetMatchingServices(0, ioServiceMatching("IOFramebuffer"), &iter) != 0 {
		return nil, errors.New("display: no framebuffers")
	}
	defer ioObjectRelease(iter)
	vendor, product, serial := cgDisplayVendorNumber(m.handle), cgDisplayModelNumber(m.handle), cgDisplaySerialNumber(m.handle)
	for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
		if !sameDisplay(entry, vendor, product, serial) {
			ioObjectRelease(entry)
			continue
		}
		for bus := uint32(0); ; bus++ {
			var iface uint32
			if ioFBCopyI2CInterfaceForBus(entry, bus, &iface) != 0 {
				break
			}
			var connect uintptr
			if ioI2CInterfaceOpen(iface, 0, &connect) == 0 {
				for _, replyType := range []uint32{i2cDDCReply, i2cSimpleTransaction} {
					d := &fbDDC{connect: connect, replyType: replyType}
					if _, _, ok := d.read(VCPBrightness); ok { // does it answer?
						ioObjectRelease(iface)
						ioObjectRelease(entry)
						return d, nil
					}
				}
				ioI2CInterfaceClose(connect, 0)
			}
			ioObjectRelease(iface)
		}
		ioObjectRelease(entry)
	}
	return nil, errors.New("display: no framebuffer I2C bus")
}

// sameDisplay compares a framebuffer's EDID identity with the display's.
func sameDisplay(entry, vendor, product, serial uint32) bool {
	info := ioDisplayCreateInfoDictionary(entry, 0)
	if info == 0 {
		return false
	}
	defer cfRelease(info)
	for key, want := range map[string]uint32{
		"DisplayVendorID": vendor, "DisplayProductID": product, "DisplaySerialNumber": serial,
	} {
		k := cfStringCreateWithCString(0, key, utf8Encoding)
		number := cfDictionaryGetValue(info, k)
		cfRelease(k)
		var got int32
		if number == 0 || !cfNumberGetValue(number, cfNumberSInt32Type, unsafe.Pointer(&got)) || uint32(got) != want {
			return false
		}
	}
	return true
}
