package mitv

import "fmt"

// NewBound refuses requests when a saved device's address belongs to a
// different neighbor, or its identity cannot currently be verified.
func NewBound(host, mac string) *Link {
	l := New(host)
	if mac != "" {
		l.checkIdentity = func() error {
			if HardwareAddress(host) != mac {
				return fmt.Errorf("mitv: device identity unavailable or changed")
			}
			return nil
		}
	}
	return l
}
