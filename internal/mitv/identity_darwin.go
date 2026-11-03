package mitv

import (
	"net"
	"syscall"
)

// HardwareAddress reads the kernel's neighbor table after discovery has
// contacted the monitor. Only a unique unicast link-layer address qualifies.
func HardwareAddress(host string) string {
	ip := net.ParseIP(host).To4()
	if ip == nil {
		return ""
	}
	rib, err := syscall.RouteRIB(syscall.NET_RT_FLAGS, syscall.RTF_LLINFO)
	if err != nil {
		return ""
	}
	messages, err := syscall.ParseRoutingMessage(rib)
	if err != nil {
		return ""
	}
	result := ""
	for _, message := range messages {
		addresses, err := syscall.ParseRoutingSockaddr(message)
		if err != nil || len(addresses) <= syscall.RTAX_GATEWAY {
			continue
		}
		dst, ok := addresses[syscall.RTAX_DST].(*syscall.SockaddrInet4)
		if !ok || !net.IP(dst.Addr[:]).Equal(ip) {
			continue
		}
		link, ok := addresses[syscall.RTAX_GATEWAY].(*syscall.SockaddrDatalink)
		if !ok {
			continue
		}
		mac := neighborMAC(link)
		if mac == "" {
			continue
		}
		if result != "" && result != mac {
			return ""
		}
		result = mac
	}
	return result
}
func neighborMAC(link *syscall.SockaddrDatalink) string {
	start := int(link.Nlen)
	if link.Alen != 6 || start+6 > len(link.Data) {
		return ""
	}
	mac := make(net.HardwareAddr, 6)
	for i := range mac {
		mac[i] = byte(link.Data[start+i])
	}
	if mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return ""
	}
	return mac.String()
}
