package mitv

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

// ErrNotFound means no HyperOS monitor answered on the local network.
var ErrNotFound = errors.New("mitv: no monitor found")

// IsMonitor tells a monitor from the other devices with the same remote
// API, such as TVs: MonitorBar must never switch a TV in another room.
func IsMonitor(name string) bool {
	return strings.Contains(name, "显示器") || strings.Contains(strings.ToLower(name), "monitor")
}

// Discover returns the IPv4 address of a HyperOS monitor by probing every
// address of the local /24 subnets for the remote API. (The device answers
// mDNS only by multicast to port 5353, which the system's own responder
// holds, so a scan is the portable way.)
func Discover() (string, error) {
	var (
		mu      sync.Mutex
		monitor string
		wg      sync.WaitGroup
	)
	for _, host := range candidates() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "6095"), 500*time.Millisecond)
			if err != nil {
				return
			}
			conn.Close()
			if name, err := New(host).DeviceName(); err == nil && IsMonitor(name) {
				mu.Lock()
				monitor = host
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if monitor == "" {
		return "", ErrNotFound
	}
	return monitor, nil
}

// candidates lists the other addresses of each private IPv4 /24 we are on.
func candidates() []string {
	var hosts []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || !ip.IsPrivate() {
			continue
		}
		for last := 1; last < 255; last++ {
			if byte(last) != ip[3] {
				hosts = append(hosts, net.IPv4(ip[0], ip[1], ip[2], byte(last)).String())
			}
		}
	}
	return hosts
}
