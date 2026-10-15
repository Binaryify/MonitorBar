package mitv

import "testing"

func TestIsMonitor(t *testing.T) {
	for name, want := range map[string]bool{
		"binaryify的REDMI 显示器 G Pro 27U 2026": true,
		"Xiaomi Monitor":         true,
		"庄煜的Redmi 智能电视 L65MA-XT": false,
		"小米电视":                   false,
		"":                       false,
	} {
		if got := IsMonitor(name); got != want {
			t.Errorf("IsMonitor(%q) = %v, want %v", name, got, want)
		}
	}
}
