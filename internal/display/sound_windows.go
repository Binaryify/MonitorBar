//go:build windows

package display

func CurrentAudioRoute() AudioRoute { return OtherAudio }
