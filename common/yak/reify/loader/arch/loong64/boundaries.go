package loong64

import (
	"sort"

	"golang.org/x/arch/loong64/loong64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/ehframe"
)

type boundaryPriority int

const (
	prioritySymbol boundaryPriority = iota
	priorityEHFrame
	priorityFunctionStart
	priorityInitArray
	priorityPLT
	priorityEntry
	priorityCall
	priorityScan
)

type loong64Boundary struct {
	core.FunctionBoundary
	priority boundaryPriority
}

type boundaryScanner struct {
	reader  *core.ByteReader
	hints   core.BoundaryHints
	image   *core.Image
	entries map[uint64]*loong64Boundary
	data    map[uint64][]byte
	stats   core.BoundaryStats
}

// discoverBoundaries finds LoongArch function starts from format metadata, symbols,
// .eh_frame, the entry point, direct calls and a prologue scan.
func discoverBoundaries(reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	if reader == nil || image == nil || len(hints.ExecSections) == 0 {
		return core.BoundaryResult{}, nil
	}
	scanner := &boundaryScanner{
		reader:  reader,
		hints:   hints,
		image:   image,
		entries: make(map[uint64]*loong64Boundary),
		data:    make(map[uint64][]byte),
	}
	scanner.addSymbols()
	scanner.addEHFrame()
	scanner.addFunctionStarts()
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

func (s *boundaryScanner) add(address, size uint64, name string, source core.BoundarySource, priority boundaryPriority) bool {
	if address == 0 || address%4 != 0 || s.regionFor(address) == nil {
		return false
	}
	// LoongArch instructions are four bytes wide; a smaller range is a marker
	// symbol, not a function.
	if size < 4 {
		size = 0
	}
	switch source {
	case core.BoundarySymbol:
		s.stats.Symbol++
	case core.BoundaryEHFrame:
		s.stats.EHFrame++
	case core.BoundaryFunctionStart:
		s.stats.FunctionStart++
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
	s.entries[address] = &loong64Boundary{
		FunctionBoundary: core.FunctionBoundary{Address: address, Size: size, Name: name, Source: source},
		priority:         priority,
	}
	return true
}

func (s *boundaryScanner) addSymbols() {
	for _, symbol := range s.image.Symbols {
		if symbol.Name == "" || symbol.Kind != sym.SymbolFunction || symbol.Imported || symbol.Address == 0 {
			continue
		}
		s.add(symbol.Address, symbol.Size, symbol.Name, core.BoundarySymbol, prioritySymbol)
	}
}

func (s *boundaryScanner) addEHFrame() {
	for _, entry := range ehframe.Entries(s.hints.EHFrame, s.hints.EHFrameAddr) {
		s.add(entry.Location, entry.Range, "", core.BoundaryEHFrame, priorityEHFrame)
	}
}

func (s *boundaryScanner) addFunctionStarts() {
	for _, address := range s.hints.FunctionStarts {
		s.add(address, 0, "", core.BoundaryFunctionStart, priorityFunctionStart)
	}
}

func (s *boundaryScanner) addInitArrays() {
	for _, address := range s.hints.InitTargets {
		s.add(address, 0, "", core.BoundaryInitArray, priorityInitArray)
	}
}

func (s *boundaryScanner) addPLT() {
	for _, entry := range s.hints.PLTTargets {
		s.add(entry.Address, 8, entry.Name, core.BoundaryPLT, priorityPLT)
	}
}

func (s *boundaryScanner) regionFor(address uint64) *core.Region {
	for index := range s.hints.ExecSections {
		region := &s.hints.ExecSections[index]
		if address >= region.Address && address-region.Address < region.Size {
			return region
		}
	}
	return nil
}

func (s *boundaryScanner) regionData(region *core.Region) []byte {
	if data, ok := s.data[region.Address]; ok {
		return data
	}
	data, _ := s.reader.Bytes(region.Offset, region.Size)
	s.data[region.Address] = data
	return data
}

func (s *boundaryScanner) decode(address uint64) (loong64asm.Inst, int, bool) {
	region := s.regionFor(address)
	if region == nil {
		return loong64asm.Inst{}, 0, false
	}
	data := s.regionData(region)
	offset := address - region.Address
	if offset+4 > uint64(len(data)) {
		return loong64asm.Inst{}, 0, false
	}
	inst, err := loong64asm.Decode(data[offset:])
	if err != nil {
		return loong64asm.Inst{}, 0, false
	}
	return inst, 4, true
}

func (s *boundaryScanner) nextBoundary(address uint64) uint64 {
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

func (s *boundaryScanner) walkDirectCalls() {
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

func (s *boundaryScanner) walkFunction(start, limit uint64) (uint64, []uint64) {
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
		inst, size, ok := s.decode(address)
		if !ok {
			continue
		}
		end := address + uint64(size)
		if end > limit {
			continue
		}
		if end > maxEnd {
			maxEnd = end
		}
		name := nameOf(inst.Op)
		switch {
		case isCall(name):
			if target, ok := branchTarget(name, args(inst), address); ok {
				calls = append(calls, target)
			}
			work = append(work, end)
		case isBranch(name):
			if target, ok := branchTarget(name, args(inst), address); ok && target >= start && target < limit {
				work = append(work, target)
			}
			work = append(work, end)
		case isTerminator(name):
			// No successors inside this function.
		default:
			work = append(work, end)
		}
	}
	return maxEnd, calls
}

func (s *boundaryScanner) finishRanges() {
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

func (s *boundaryScanner) addTailScan() {
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

// scanGap recovers a function start that no symbol reached. LoongArch
// instructions are fixed width, so every aligned slot is a candidate; the scan
// trusts a slot only when the decoder accepts it and the instruction can start a
// function.
func (s *boundaryScanner) scanGap(start, end uint64) {
	padding := true
	for address := start; address+8 <= end; {
		inst, size, ok := s.decode(address)
		if !ok {
			padding = true
			address += 4
			continue
		}
		name := nameOf(inst.Op)
		if isTerminator(name) {
			padding = true
			address += uint64(size)
			continue
		}
		if padding && plausibleFunctionStart(name) {
			s.add(address, 0, "", core.BoundaryScan, priorityScan)
			padding = false
			address += uint64(size)
			continue
		}
		padding = false
		address += uint64(size)
	}
}

// plausibleFunctionStart accepts the prologue forms the compilers emit: a stack
// adjustment, a saved register or an address load.
func plausibleFunctionStart(name string) bool {
	switch name {
	case "addi.d", "addi.w", "st.d", "st.w", "or", "move", "pcaddu12i",
		"pcalau12i", "lu12i.w", "ld.d", "ld.w", "b":
		return true
	}
	return false
}
