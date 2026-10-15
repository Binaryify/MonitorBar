//go:build windows

package display

// Sharing a monitor between two computers needs the per-display calls
// below; they exist on macOS only for now.

func OnlineScreens() []Screen { return nil }

func SetScreenEnabled(id uint32, on bool) error { return ErrUnsupported }

func OpenScreenDDC(id uint32) (*DDC, error) { return nil, ErrUnsupported }
