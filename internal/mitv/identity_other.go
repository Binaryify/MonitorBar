//go:build !darwin

package mitv

// Other platforms retain manual address updates until a neighbor reader exists.
func HardwareAddress(host string) string { return "" }
