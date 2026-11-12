package amd64

import (
	"sort"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

type boundaryPriority int

const (
	prioritySymbol boundaryPriority = iota
	priorityEHFrame
	priorityInitArray
	priorityPLT
	priorityEntry
	priorityCall
	priorityScan
)

type amd64Boundary struct {
	core.FunctionBoundary
	priority boundaryPriority
}

type amd64BoundaryScanner struct {
	reader  *core.ByteReader
	hints   core.BoundaryHints
	image   *core.Image
	entries map[uint64]*amd64Boundary
	data    map[uint64][]byte
	stats   core.BoundaryStats
}

// DiscoverBoundaries finds function starts in an AMD64 image using loader
// hints, symbols, entry, direct calls and tail scanning. It does not build
// LowIR; asm2lowir uses the result to call Parse per function.
func (Parser) DiscoverBoundaries(reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	if reader == nil || image == nil || len(hints.ExecSections) == 0 {
		return core.BoundaryResult{}, nil
	}
	scanner := &amd64BoundaryScanner{
		reader:  reader,
		hints:   hints,
		image:   image,
		entries: make(map[uint64]*amd64Boundary),
		data:    make(map[uint64][]byte),
	}
	scanner.addSymbols()
	scanner.addEHFrame()
	scanner.addInitArrays()
	scanner.addPLT()
	if entry := image.Entry; entry != nil {
		scanner.add(*entry, 0, "", core.BoundaryEntry, priorityEntry)
	}
	scanner.walkDirectCalls()
	scanner.finishRanges()
	scanner.addTailScan()
	scanner.finishRanges()

	functions := make([]core.FunctionBoundary, 0, len(scanner.entries))
	for _, boundary := range scanner.entries {
		functions = append(functions, boundary.FunctionBoundary)
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i].Address < functions[j].Address })
	return core.BoundaryResult{Functions: functions, Stats: scanner.stats}, nil
}

func (s *amd64BoundaryScanner) add(address, size uint64, name string, source core.BoundarySource, priority boundaryPriority) bool {
	if address == 0 || s.regionFor(address) == nil {
		return false
	}
	switch source {
	case core.BoundarySymbol:
		s.stats.Symbol++
	case core.BoundaryEHFrame:
		s.stats.EHFrame++
	case core.BoundaryInitArray:
		s.stats.InitArray++
	case core.BoundaryPLT:
		s.stats.PLT++
	case core.BoundaryEntry:
		s.stats.Entry++
	case core.BoundaryCall:
		s.stats.Call++
	case core.BoundaryScan:
		s.stats.Scan++
	}
	if existing, ok := s.entries[address]; ok {
		s.stats.Duplicates++
		if priority < existing.priority {
			existing.Size = size
			existing.Name = name
			existing.Source = source
			existing.priority = priority
		} else {
			if existing.Size == 0 && size != 0 {
				existing.Size = size
			}
			if existing.Name == "" && name != "" {
				existing.Name = name
			}
		}
		return false
	}
	s.entries[address] = &amd64Boundary{
		FunctionBoundary: core.FunctionBoundary{Address: address, Size: size, Name: name, Source: source},
		priority:         priority,
	}
	return true
}

func (s *amd64BoundaryScanner) addSymbols() {
	for _, symbol := range s.image.Symbols {
		if symbol.Name == "" || symbol.Kind != sym.SymbolFunction || symbol.Imported || symbol.Address == 0 {
			continue
		}
		s.add(symbol.Address, symbol.Size, symbol.Name, core.BoundarySymbol, prioritySymbol)
	}
}

func (s *amd64BoundaryScanner) addPLT() {
	for _, entry := range s.hints.PLTTargets {
		s.add(entry.Address, 16, entry.Name, core.BoundaryPLT, priorityPLT)
	}
}

func (s *amd64BoundaryScanner) addInitArrays() {
	for _, address := range s.hints.InitTargets {
		s.add(address, 0, "", core.BoundaryInitArray, priorityInitArray)
	}
}

func (s *amd64BoundaryScanner) regionFor(address uint64) *core.Region {
	for index := range s.hints.ExecSections {
		region := &s.hints.ExecSections[index]
		if address >= region.Address && address-region.Address < region.Size {
			return region
		}
	}
	return nil
}

func (s *amd64BoundaryScanner) regionData(region *core.Region) []byte {
	if data, ok := s.data[region.Address]; ok {
		return data
	}
	data, _ := s.reader.Bytes(region.Offset, region.Size)
	s.data[region.Address] = data
	return data
}

func (s *amd64BoundaryScanner) decode(address uint64) (x86asm.Inst, bool) {
	region := s.regionFor(address)
	if region == nil {
		return x86asm.Inst{}, false
	}
	data := s.regionData(region)
	offset := address - region.Address
	if offset >= uint64(len(data)) {
		return x86asm.Inst{}, false
	}
	if offset+4 <= uint64(len(data)) && data[offset] == 0xf3 && data[offset+1] == 0x0f && data[offset+2] == 0x1e && (data[offset+3] == 0xfa || data[offset+3] == 0xfb) {
		return x86asm.Inst{Op: x86asm.NOP, Len: 4}, true
	}
	inst, err := x86asm.Decode(data[offset:], 64)
	if err != nil || inst.Len <= 0 {
		return x86asm.Inst{}, false
	}
	return inst, true
}

func (s *amd64BoundaryScanner) nextBoundary(address uint64) uint64 {
	next := uint64(0)
	for candidate := range s.entries {
		if candidate > address && (next == 0 || candidate < next) {
			next = candidate
		}
	}
	if region := s.regionFor(address); region != nil {
		end := region.Address + region.Size
		if next == 0 || next > end {
			return end
		}
	}
	return next
}

func (s *amd64BoundaryScanner) walkDirectCalls() {
	queue := make([]uint64, 0, len(s.entries))
	for address := range s.entries {
		queue = append(queue, address)
	}
	sort.Slice(queue, func(i, j int) bool { return queue[i] < queue[j] })
	processed := make(map[uint64]bool)
	for len(queue) > 0 {
		start := queue[0]
		queue = queue[1:]
		if processed[start] {
			continue
		}
		processed[start] = true
		if s.regionFor(start) == nil {
			continue
		}
		limit := s.nextBoundary(start)
		if entry, ok := s.entries[start]; ok && entry.Size > 0 {
			if end := start + entry.Size; end < limit {
				limit = end
			}
		}
		end, calls := s.walkFunction(start, limit)
		if end > start {
			if entry, ok := s.entries[start]; ok {
				if entry.Size == 0 || end-start > entry.Size {
					entry.Size = end - start
				}
			}
		}
		for _, target := range calls {
			if _, exists := s.entries[target]; exists || processed[target] {
				continue
			}
			if s.add(target, 0, "", core.BoundaryCall, priorityCall) {
				queue = append(queue, target)
			}
		}
	}
}

func (s *amd64BoundaryScanner) walkFunction(start, limit uint64) (uint64, []uint64) {
	work := []uint64{start}
	visited := make(map[uint64]bool)
	calls := make([]uint64, 0)
	maxEnd := start
	for len(work) > 0 {
		address := work[len(work)-1]
		work = work[:len(work)-1]
		if address < start || address >= limit || visited[address] {
			continue
		}
		visited[address] = true
		inst, ok := s.decode(address)
		if !ok {
			continue
		}
		end := address + uint64(inst.Len)
		if end > limit {
			continue
		}
		if end > maxEnd {
			maxEnd = end
		}
		next := end
		switch inst.Op {
		case x86asm.CALL:
			if target, ok := directX86Target(inst, address); ok {
				calls = append(calls, target)
			} else if target, ok := s.indirectGOTTarget(inst, address); ok {
				calls = append(calls, target)
			}
		case x86asm.JMP:
			if target, ok := directX86Target(inst, address); ok {
				if target >= start && target < limit {
					work = append(work, target)
					continue
				}
				continue
			}
			continue
		case x86asm.RET, x86asm.UD2, x86asm.HLT:
			continue
		case x86asm.INT:
			if imm, ok := inst.Args[0].(x86asm.Imm); ok && imm == 3 {
				continue
			}
		default:
			if isConditionalJump(inst.Op) {
				if target, ok := directX86Target(inst, address); ok {
					work = append(work, target)
				}
				work = append(work, next)
				continue
			}
		}
		work = append(work, next)
	}
	return maxEnd, calls
}

func directX86Target(inst x86asm.Inst, address uint64) (uint64, bool) {
	if len(inst.Args) == 0 {
		return 0, false
	}
	switch arg := inst.Args[0].(type) {
	case x86asm.Rel:
		return address + uint64(inst.Len) + uint64(int64(arg)), true
	case x86asm.Imm:
		return uint64(int64(arg)), true
	default:
		return 0, false
	}
}

func (s *amd64BoundaryScanner) indirectGOTTarget(inst x86asm.Inst, address uint64) (uint64, bool) {
	if len(inst.Args) == 0 {
		return 0, false
	}
	mem, ok := inst.Args[0].(x86asm.Mem)
	if !ok || mem.Base != x86asm.RIP {
		return 0, false
	}
	got := address + uint64(inst.Len) + uint64(int64(mem.Disp))
	target, ok := s.hints.GOTRelocs[got]
	return target, ok && target != 0
}

func isConditionalJump(op x86asm.Op) bool {
	switch op {
	case x86asm.JA, x86asm.JAE, x86asm.JB, x86asm.JBE, x86asm.JCXZ, x86asm.JECXZ, x86asm.JG, x86asm.JGE, x86asm.JL, x86asm.JLE, x86asm.JNE, x86asm.JNO, x86asm.JNP, x86asm.JNS, x86asm.JO, x86asm.JP, x86asm.JRCXZ, x86asm.JS:
		return true
	default:
		return false
	}
}

func (s *amd64BoundaryScanner) finishRanges() {
	addresses := make([]uint64, 0, len(s.entries))
	for address := range s.entries {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
	for _, address := range addresses {
		entry := s.entries[address]
		if entry.Size != 0 {
			continue
		}
		if next := s.nextBoundary(address); next > address {
			entry.Size = next - address
		}
	}
}

func (s *amd64BoundaryScanner) addTailScan() {
	covered := make([]core.FunctionBoundary, 0, len(s.entries))
	for _, entry := range s.entries {
		covered = append(covered, entry.FunctionBoundary)
	}
	sort.Slice(covered, func(i, j int) bool { return covered[i].Address < covered[j].Address })
	for _, region := range s.hints.ExecSections {
		start := region.Address
		end := region.Address + region.Size
		for _, function := range covered {
			if function.Address < start || function.Address >= end {
				continue
			}
			funcEnd := function.Address + function.Size
			if funcEnd > end {
				funcEnd = end
			}
			if funcEnd > start {
				start = funcEnd
			}
		}
		s.scanGap(start, end)
	}
}

func (s *amd64BoundaryScanner) scanGap(start, end uint64) {
	for address := start; address+16 <= end; {
		region := s.regionFor(address)
		if region == nil {
			return
		}
		data := s.regionData(region)
		offset := address - region.Address
		if offset >= uint64(len(data)) {
			return
		}
		if data[offset] == 0xcc || data[offset] == 0x90 || data[offset] == 0x00 {
			address++
			continue
		}
		if address%16 != 0 {
			address++
			continue
		}
		inst, ok := s.decode(address)
		if !ok || inst.Op == x86asm.NOP || inst.Op == x86asm.INT {
			address++
			continue
		}
		s.add(address, end-address, "", core.BoundaryScan, priorityScan)
		address += 16
	}
}
