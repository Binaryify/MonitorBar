//go:build windows

package display

// SoundToDisplay has no Windows counterpart yet: the media keys are
// macOS's alone.
func SoundToDisplay() bool { return false }

func ActiveDisplayAudioUIDs() []string { return nil }
