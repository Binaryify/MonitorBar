package main

import (
	"math"
	"testing"
	"time"
)

func TestFollowFastGlideNoOvershoot(t *testing.T) {
	for _, pair := range [][2]float64{{0, 100}, {100, 0}, {40, 42}, {42, 40}} {
		v, target := pair[0], pair[1]
		for range int(600 * time.Millisecond / followInterval) {
			next := easeToward(v, target)
			if next < math.Min(v, target) || next > math.Max(v, target) {
				t.Fatal("overshoot")
			}
			if math.Abs(target-v) > 1 && math.Abs(next-v) >= math.Abs(target-v) {
				t.Fatal("not a glide")
			}
			v = next
		}
		if v != target {
			t.Fatalf("did not settle: %v → %v", v, target)
		}
	}
	gain := 1 - math.Exp(-float64(followInterval)/float64(100*time.Millisecond))
	if got := easeToward(40, 42); math.Abs(got-(40+2*gain)) > 1e-10 || got-40 >= 1 {
		t.Fatal("minimum step returned")
	}
}
