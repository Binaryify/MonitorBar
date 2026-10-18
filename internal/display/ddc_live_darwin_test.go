//go:build darwin

package display

import (
	"os"
	"testing"
	"time"
)

// TestLiveInputSwitch talks to the real monitor through whichever DDC/CI path
// this Mac has, and switches the input away and back. It is skipped by default
// because it changes what the panel shows: set MONITORBAR_LIVE_DDC=1 to run it.
func TestLiveInputSwitch(t *testing.T) {
	if os.Getenv("MONITORBAR_LIVE_DDC") == "" {
		t.Skip("set MONITORBAR_LIVE_DDC=1 to talk to the monitor")
	}
	m := Target()
	t.Logf("monitor: %q, connection %q", m.Name, m.Connection)
	d, err := m.OpenDDC()
	if err != nil {
		t.Fatalf("OpenDDC: %v", err)
	}
	defer d.Close()

	cur, max, ok := d.Get(VCPInput)
	if !ok {
		t.Fatal("the monitor doesn't answer VCP 0x60 (input source)")
	}
	t.Logf("input source: 0x%02X (%s), max %d", cur, InputNameFor(m.Name, cur), max)
	caps := d.Capabilities()
	t.Logf("capabilities: %q", caps)
	t.Logf("inputs in the capability string: %v", ParseCapabilities(caps)[VCPInput])

	other := 0x11 // HDMI 1, unless that is where we are
	if cur == other {
		other = 0x0F
	}
	defer func() {
		d.Set(VCPInput, cur) // put the picture back whatever happens
		waitForInput(t, d, cur, 5*time.Second)
	}()
	d.Set(VCPInput, other)
	if !waitForInput(t, d, other, 5*time.Second) {
		t.Fatalf("input source did not change to 0x%02X", other)
	}
	t.Logf("switched to 0x%02X (%s)", other, InputNameFor(m.Name, other))
}

// waitForInput polls until the monitor reports the input asked for.
func waitForInput(t *testing.T, d *DDC, want int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got, _, ok := d.Get(VCPInput); ok && got == want {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
