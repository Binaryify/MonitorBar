//go:build windows

package display

// CurrentAudioRoute returns OtherAudio on Windows, where media-key interception is unsupported.
func CurrentAudioRoute() AudioRoute { return OtherAudio }
