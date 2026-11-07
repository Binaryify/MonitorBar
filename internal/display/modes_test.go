package display

import (
	"slices"
	"testing"
)

func TestModeIdentityAndGrouping(t *testing.T) {
	base := Mode{Width: 1920, Height: 1080, PixelWidth: 3840, PixelHeight: 2160, RefreshRate: 60}
	backing := base
	backing.PixelWidth, backing.PixelHeight = 5120, 2880
	fast := base
	fast.RefreshRate = 120
	if base.ID() == backing.ID() || base.SizeKey() == backing.SizeKey() || base.ID() == fast.ID() || base.SizeKey() != fast.SizeKey() {
		t.Fatal("identity must distinguish backing size and rate; resolution groups only rates")
	}
	modes := []Mode{base, backing, fast, base}
	if got := dedupe(slices.Clone(modes)); len(got) != 3 {
		t.Fatalf("distinct modes after deduplication: %v", got)
	}
	if got := Resolutions(modes, true, 60); len(got) != 2 || got[0] != backing || got[1] != base {
		t.Fatalf("resolutions must keep both backing sizes and prefer 60 Hz: %v", got)
	}
	if got := Resolutions(modes, true, 90); len(got) != 2 || got[1] != fast {
		t.Fatalf("an unavailable rate must use the fastest: %v", got)
	}
	if got := RefreshRates(dedupe(slices.Clone(modes)), base); !slices.Equal(got, []Mode{fast, base}) {
		t.Fatalf("refresh rates must stay within the exact backing size: %v", got)
	}
	standard := base
	standard.PixelWidth, standard.PixelHeight = standard.Width, standard.Height
	if got := Resolutions(append(modes, standard), false, 60); !slices.Equal(got, []Mode{standard}) {
		t.Fatalf("standard resolutions: %v", got)
	}
}
