package display

import "testing"

func TestArrangePutsExternalsFirst(t *testing.T) {
	got := arrange([]Monitor{
		{ID: "built-in", Builtin: true, x: 0},
		{ID: "right", x: 1920},
		{ID: "left", x: 0},
	})
	want := []string{"left", "right", "built-in"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("order %v, want %v", ids(got), want)
		}
	}
}

func TestUniqueIDsSuffixRepeats(t *testing.T) {
	got := uniqueIDs([]Monitor{{ID: "a"}, {ID: "a"}, {ID: "b"}})
	want := []string{"a", "a#2", "b"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("ids %v, want %v", ids(got), want)
		}
	}
}

func ids(monitors []Monitor) []string {
	out := make([]string, len(monitors))
	for i, m := range monitors {
		out[i] = m.ID
	}
	return out
}
