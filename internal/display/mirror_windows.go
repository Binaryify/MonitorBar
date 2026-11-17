//go:build windows

package display

func (m Monitor) InputMirror(mac, windows int) *InputMirror { return nil }
func (m Monitor) Mirrored() bool                            { return false }
