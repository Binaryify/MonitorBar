//go:build !darwin && !windows

package keystate

func down(int) bool { return false }

var codes = map[string][]int{}
