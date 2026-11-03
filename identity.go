package main

import "maps"

// addressMoves uses only unique MACs, never names or model numbers. A
// destination occupied by an unmatched profile is left alone for manual repair.
func addressMoves(saved, found map[string]string) map[string]string {
	moves := map[string]string{}
	for old, mac := range saved {
		if mac == "" {
			continue
		}
		oldCount, newCount, next := 0, 0, ""
		for _, m := range saved {
			if m == mac {
				oldCount++
			}
		}
		for h, m := range found {
			if m == mac {
				newCount++
				next = h
			}
		}
		if oldCount == 1 && newCount == 1 && next != old {
			moves[old] = next
		}
	}
	for {
		changed := false
		for old, next := range moves {
			if _, occupied := saved[next]; occupied {
				if _, moving := moves[next]; !moving {
					delete(moves, old)
					changed = true
				}
			}
		}
		if !changed {
			return moves
		}
	}
}

// Remap from a snapshot so two devices can exchange addresses without
// overwriting each other's names, audio routes, screen bindings or presets.
func remapKeys[V any](values map[string]V, moves map[string]string) {
	before := maps.Clone(values)
	for old := range moves {
		delete(values, old)
	}
	for old, next := range moves {
		if v, ok := before[old]; ok {
			values[next] = v
		}
	}
}
func (s *settings) remapDevices(moves map[string]string) {
	remapKeys(s.DeviceMACs, moves)
	remapKeys(s.DeviceNames, moves)
	remapKeys(s.AudioBindings, moves)
	remapKeys(s.ScreenBindings, moves)
	remapKeys(s.ExcludedAudio, moves)
	for name, p := range s.Presets {
		remapKeys(p.Volumes, moves)
		s.Presets[name] = p
	}
	for i, h := range s.KnownHosts {
		if next, ok := moves[h]; ok {
			s.KnownHosts[i] = next
		}
	}
	if next, ok := moves[s.NetworkHost]; ok {
		s.NetworkHost = next
	}
}
