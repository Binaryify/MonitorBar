package mitv

import (
	"errors"
	"testing"
)

func TestChaseVolumeLatestTarget(t *testing.T) {
	actual, target, calls := 20, 30, 0
	v, err := chaseVolume(func() (int, error) { return actual, nil }, func(key string) error {
		calls++
		if key == "volumeup" {
			actual++
		} else {
			actual--
		}
		if calls == 2 {
			target = 19
		}
		return nil
	}, func() int { return target })
	if err != nil || v != 19 || calls != 5 {
		t.Fatalf("v=%d calls=%d err=%v", v, calls, err)
	}
}
func TestChaseVolumeStopsOnFailure(t *testing.T) {
	calls := 0
	_, err := chaseVolume(func() (int, error) { return 20, nil }, func(string) error { calls++; return errors.New("offline") }, func() int { return 50 })
	if err == nil || calls != 1 {
		t.Fatal("failed commands retried")
	}
}
func TestChaseVolumeRejectsNoProgress(t *testing.T) {
	calls := 0
	_, err := chaseVolume(func() (int, error) { return 20, nil }, func(string) error { calls++; return nil }, func() int { return 50 })
	if err == nil || calls != 1 {
		t.Fatal("non-responsive control loop")
	}
}
