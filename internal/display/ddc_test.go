package display

import (
	"slices"
	"sync"
	"testing"
	"time"
)

func TestParseCapabilities(t *testing.T) {
	caps := "(prot(monitor)type(LCD)model(X)cmds(01 02 03 F3)vcp(02 04 10 12 14(05 08 0B) 60(0F 11 12 1B) 62 8D(01 02))mccs_ver(2.2))"
	got := ParseCapabilities(caps)
	if !slices.Equal(got[VCPInput], []int{0x0F, 0x11, 0x12, 0x1B}) {
		t.Errorf("inputs = %v", got[VCPInput])
	}
	if _, ok := got[VCPBrightness]; !ok {
		t.Error("brightness missing")
	}
	if !slices.Equal(ParseCapabilities("vcp(101260(0F11))")[VCPInput], []int{0x0F, 0x11}) {
		t.Error("unspaced string")
	}
	if ParseCapabilities("(prot(monitor))") != nil {
		t.Error("no vcp section should give nil")
	}
}

// fakeTransport records writes, to check that Set keeps only the latest value.
type fakeTransport struct {
	mu     sync.Mutex
	writes [][2]int
	closes int
}

func (f *fakeTransport) read(code byte) (int, int, bool) { return 50, 100, true }
func (f *fakeTransport) write(code byte, v int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, [2]int{int(code), v})
	return true
}
func (f *fakeTransport) capabilities() string { return "" }
func (f *fakeTransport) close()               { f.mu.Lock(); defer f.mu.Unlock(); f.closes++ }

func TestSetCoalesces(t *testing.T) {
	f := &fakeTransport{}
	d := &DDC{t: f, pending: map[byte]int{}, wake: make(chan struct{}, 1), done: make(chan struct{})}

	d.ioMu.Lock() // block bus I/O, not producers
	go d.writer()
	for v := range 50 {
		d.Set(VCPBrightness, v)
	}
	d.Set(VCPBrightness, 80)
	d.ioMu.Unlock()
	for range 200 {
		time.Sleep(5 * time.Millisecond)
		f.mu.Lock()
		n := len(f.writes)
		f.mu.Unlock()
		if n > 0 {
			break
		}
	}

	d.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 1 || f.writes[0] != [2]int{int(VCPBrightness), 80} {
		t.Errorf("writes = %v, want one write of 80", f.writes)
	}
}

func TestInputNameFor(t *testing.T) {
	if got := InputNameFor("Redmi 27 NU", 0x10); got != "USB-C" {
		t.Errorf("Redmi 27 NU 0x10 = %q, want USB-C", got)
	}
	if got := InputNameFor("Redmi 27 NU", 0x11); got != "HDMI 1" {
		t.Errorf("Redmi 27 NU 0x11 = %q, want HDMI 1", got)
	}
	if got := InputNameFor("Some Monitor", 0x10); got != "DP 2" {
		t.Errorf("other models 0x10 = %q, want DP 2", got)
	}
}

func TestUniqueKeys(t *testing.T) {
	got := uniqueKeys([]Monitor{{Key: "a"}, {Key: "b"}, {Key: "a"}, {Key: "a"}})
	want := []string{"a", "b", "a#2", "a#3"}
	for i, m := range got {
		if m.Key != want[i] {
			t.Errorf("key %d: %q, want %q", i, m.Key, want[i])
		}
	}
}

func TestCloseIgnoresLateProducers(t *testing.T) {
	f := &fakeTransport{}
	d := &DDC{t: f, pending: map[byte]int{}, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go d.writer()
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 30 {
				d.Set(VCPBrightness, 40)
				d.Get(VCPInput)
				d.Capabilities()
			}
			d.Close()
		})
	}
	wg.Wait()
	d.Close()
	d.Set(VCPBrightness, 80)
	if _, _, ok := d.Get(VCPInput); ok {
		t.Fatal("read after close")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closes != 1 {
		t.Fatalf("closed %d times", f.closes)
	}
}
