package peer

import "testing"

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/Applications/MonitorBar.app/Contents/MacOS/MonitorBar": "'/Applications/MonitorBar.app/Contents/MacOS/MonitorBar'",
		"it's": `'it'\''s'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLastLine(t *testing.T) {
	if got := lastLine("Welcome\n{\"held\":true}\n"); got != `{"held":true}` {
		t.Errorf("lastLine = %q", got)
	}
}

func TestStatusHas(t *testing.T) {
	s := Status{Screens: []Screen{{Key: "25001:12300", Active: true}, {Key: "25001:10163"}}}
	if !s.Has("25001:12300") || s.Has("25001:10163") || s.Has("1:2") {
		t.Errorf("Has is wrong for %+v", s)
	}
}

func TestNoHost(t *testing.T) {
	if _, err := (SSH{}).Status(); err != ErrNoHost {
		t.Errorf("err = %v, want ErrNoHost", err)
	}
}
