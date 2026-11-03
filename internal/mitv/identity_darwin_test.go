package mitv

import (
	"syscall"
	"testing"
)

func TestNeighborMAC(t *testing.T) {
	link := &syscall.SockaddrDatalink{Nlen: 3, Alen: 6}
	for i, b := range []byte{0x9c, 0x12, 0x21, 0x8c, 0x8a, 0xc3} {
		link.Data[3+i] = int8(b)
	}
	if got := neighborMAC(link); got != "9c:12:21:8c:8a:c3" {
		t.Fatal(got)
	}
	link.Nlen = 255
	if neighborMAC(link) != "" {
		t.Fatal("invalid offset accepted")
	}
	link.Nlen = 3
	link.Alen = 0
	if neighborMAC(link) != "" {
		t.Fatal("incomplete neighbor accepted")
	}
	link.Alen = 6
	link.Data[3] = int8(-1)
	if neighborMAC(link) != "" {
		t.Fatal("multicast accepted")
	}
}
