//go:build !darwin

package mediakeys

// Supported says whether this system can take the keys.
const Supported = false

// SettingsURL has no counterpart without the Accessibility permission.
const SettingsURL = ""

func Trusted() bool       { return false }
func RequestAccess()      {}
func Running() bool       { return false }
func Start(Handler) error { return ErrNotTrusted }
func Stop()               {}
