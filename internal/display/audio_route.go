package display

type AudioRoute uint8

const (
	OtherAudio AudioRoute = iota
	DisplayAudio
	GroupDisplayAudio
)

// fourCC packs a Core Audio four-character code.
func fourCC(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

// An aggregate counts only when one of its active members outputs to a display.
func classifyAudioRoute(root uint32, transport func(uint32) uint32, members func(uint32) []uint32) AudioRoute {
	seen := map[uint32]bool{}
	var visit func(uint32) AudioRoute
	visit = func(id uint32) AudioRoute {
		if id == 0 || seen[id] {
			return OtherAudio
		}
		seen[id] = true
		switch transport(id) {
		case fourCC("hdmi"), fourCC("dprt"):
			return DisplayAudio
		case fourCC("grup"):
			for _, child := range members(id) {
				if visit(child) != OtherAudio {
					return GroupDisplayAudio
				}
			}
		}
		return OtherAudio
	}
	return visit(root)
}
