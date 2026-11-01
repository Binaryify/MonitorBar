package mitv

import "errors"

// ChaseVolume coalesces held keys into one latest target and verifies every
// acknowledged step. It never queues a replay after a network error.
func (l *Link) ChaseVolume(target func() int) (int, error) {
	return chaseVolume(l.Volume, l.Press, target)
}
func chaseVolume(read func() (int, error), press func(string) error, target func() int) (int, error) {
	actual, err := read()
	if err != nil {
		return 0, err
	}
	for steps := 0; steps < 200; steps++ {
		want := min(max(target(), 0), 100)
		if want == actual {
			return actual, nil
		}
		key := "volumeup"
		if want < actual {
			key = "volumedown"
		}
		if err := press(key); err != nil {
			return actual, err
		}
		next, err := read()
		if err != nil {
			return actual, err
		}
		if next == actual {
			return actual, errors.New("volume command did not change device state")
		}
		actual = next
	}
	return actual, errors.New("volume adjustment exceeded step limit")
}
