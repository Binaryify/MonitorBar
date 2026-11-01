package display

// outputContainsDisplay follows active aggregate members, including Multi-Output
// Devices. A visited set also handles malformed or cyclic device graphs.
func outputContainsDisplay(root uint32, read func(uint32) (string, []uint32)) bool {
	seen := map[uint32]bool{}
	var visit func(uint32) bool
	visit = func(id uint32) bool {
		if id == 0 || seen[id] {
			return false
		}
		seen[id] = true
		t, members := read(id)
		if t == "hdmi" || t == "dprt" {
			return true
		}
		if t == "grup" {
			for _, child := range members {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(root)
}
