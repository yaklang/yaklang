package arm

import (
	"sort"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// decodeAny decodes one instruction in the mode that address implies. It returns
// the instruction, its size and the mode.
func (s *boundaryScanner) decodeAny(address uint64) (armasm.Inst, int, bool) {
	if s.modeFor(address) == "thumb" {
		region := s.regionFor(address)
		if region == nil {
			return armasm.Inst{}, 0, false
		}
		data := s.regionData(region)
		offset := address - region.Address
		if offset+2 > uint64(len(data)) {
			return armasm.Inst{}, 0, false
		}
		size := 4
		if offset+4 > uint64(len(data)) {
			size = 2
		}
		decoded, decodedSize := decodeThumb(data[offset:offset+uint64(size)], &thumbState{})
		if decoded.undefined {
			return armasm.Inst{}, 0, false
		}
		return decoded.inst, decodedSize, true
	}
	inst, ok := s.decode(address)
	if !ok {
		return armasm.Inst{}, 0, false
	}
	return inst, 4, true
}

// modeFor returns the encoding mode recorded for one address. A boundary
// records its own mode; a range whose format records a mode overrides the
// default; an address inside a function inherits the mode of the nearest
// boundary at or below it.
func (s *boundaryScanner) modeFor(address uint64) string {
	if entry, ok := s.entries[address]; ok && entry.Mode != "" {
		return entry.Mode
	}
	for _, region := range s.hints.ModeRegions {
		if address >= region.Address && address-region.Address < region.Size {
			return region.Mode
		}
	}
	best := uint64(0)
	mode := ""
	for candidate, entry := range s.entries {
		if candidate > address || entry.Mode == "" {
			continue
		}
		if candidate >= best {
			best = candidate
			mode = entry.Mode
		}
	}
	if mode == "" {
		return s.defaultMode
	}
	return mode
}

// markThumbFromSymbols records the mode of every boundary that an odd symbol
// address identifies.
func (s *boundaryScanner) markThumbFromSymbols() {
	for _, symbol := range s.image.Symbols {
		if symbol.Address == 0 || symbol.Name == "" {
			continue
		}
		address := symbol.Address &^ 1
		if entry, ok := s.entries[address]; ok && symbol.Address%2 == 1 {
			entry.Mode = "thumb"
		}
	}
}

// walkThumbCalls recovers Thumb functions by recursive descent. A stripped
// image has no symbols, so the call graph is the only reliable source: every BL
// target is a function, and the walk stops when the fall-through reaches
// another function.
func (s *boundaryScanner) walkThumbCalls() {
	for pass := 0; pass < 8; pass++ {
		addresses := make([]uint64, 0, len(s.entries))
		for address := range s.entries {
			addresses = append(addresses, address)
		}
		sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
		added := false
		for _, address := range addresses {
			entry, ok := s.entries[address]
			if !ok || entry.Mode != "thumb" {
				continue
			}
			limit := s.nextBoundary(address)
			if entry.Size > 0 {
				if end := address + entry.Size; end < limit {
					limit = end
				}
			}
			if s.walkThumbFunction(address, limit) {
				added = true
			}
		}
		if !added {
			return
		}
	}
}

// walkThumbFunction walks one Thumb function and records its direct call
// targets. It reports whether it discovered a new function.
func (s *boundaryScanner) walkThumbFunction(start, limit uint64) bool {
	work := []uint64{start}
	visited := make(map[uint64]bool)
	discovered := false
	for len(work) > 0 {
		address := work[len(work)-1]
		work = work[:len(work)-1]
		if address < start || address >= limit || visited[address] {
			continue
		}
		visited[address] = true
		inst, size, ok := s.decodeAny(address)
		if !ok {
			continue
		}
		end := address + uint64(size)
		if end > limit {
			continue
		}
		base := baseOp(inst.Op)
		if base == armasm.BL || base == armasm.BLX {
			if target, ok := thumbStaticTarget(inst, address, size); ok {
				if _, exists := s.entries[target]; !exists {
					// A call target follows the encoding of the range it sits
					// in, which is not always Thumb: a Thumb function calls the
					// ARM PLT through the same instruction.
					if s.addWithMode(target, 0, "", core.BoundaryCall, priorityCall, s.modeFor(target)) {
						discovered = true
					}
				}
			}
			work = append(work, end)
			continue
		}
		if base == armasm.B {
			if target, ok := thumbStaticTarget(inst, address, size); ok {
				if target >= start && target < limit {
					work = append(work, target)
				}
			}
			continue
		}
		if base == armasm.BX {
			continue
		}
		// A conditional branch keeps both successors.
		work = append(work, end)
	}
	return discovered
}

// thumbStaticTarget returns the branch target of a Thumb instruction. A Thumb
// branch offset is relative to the instruction address plus four, and the
// decoder stores it that way.
func thumbStaticTarget(inst armasm.Inst, address uint64, size int) (uint64, bool) {
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		offset, ok := pcRelative(arg)
		if !ok {
			continue
		}
		return address + 4 + uint64(offset), true
	}
	return 0, false
}

// addWithMode records one boundary together with its encoding mode. An existing
// record keeps the mode of its strongest source: a symbol or a PLT stub decides
// the mode, a scanned candidate only fills it in.
func (s *boundaryScanner) addWithMode(address, size uint64, name string, source core.BoundarySource, priority boundaryPriority, mode string) bool {
	if address == 0 || s.regionFor(address) == nil {
		return false
	}
	if size < 2 {
		size = 0
	}
	aligned := address &^ 1
	wasNew := s.add(aligned, size, name, source, priority)
	entry, ok := s.entries[aligned]
	if !ok {
		return wasNew
	}
	if wasNew || entry.Mode == "" || priority <= entry.priority {
		entry.Mode = mode
	}
	return wasNew
}
