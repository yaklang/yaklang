package arm

import (
	"sort"
	"strings"

	"golang.org/x/arch/arm/armasm"

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

type armBoundary struct {
	core.FunctionBoundary
	priority boundaryPriority
}

type boundaryScanner struct {
	reader  *core.ByteReader
	hints   core.BoundaryHints
	image   *core.Image
	entries map[uint64]*armBoundary
	data    map[uint64][]byte
	stats   core.BoundaryStats
}

// DiscoverBoundaries finds 32-bit ARM function starts. Format metadata, symbols,
// .eh_frame, the entry point, direct BL targets and a prologue scan contribute
// candidates. It never builds LowIR; asm2lowir calls Parse once per function.
func (Parser) DiscoverBoundaries(reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	if reader == nil || image == nil || len(hints.ExecSections) == 0 {
		return core.BoundaryResult{}, nil
	}
	scanner := &boundaryScanner{
		reader:  reader,
		hints:   hints,
		image:   image,
		entries: make(map[uint64]*armBoundary),
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
	// ARM instructions are four bytes wide. ELF also carries marker symbols
	// such as go:textfipsstart whose size is one byte; a size smaller than one
	// instruction is not a real range, so let the next boundary decide it.
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
	s.entries[address] = &armBoundary{
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
		name := symbol.Name
		// ARM ELF marks Thumb symbols by setting the low bit of st_value.
		if symbol.Address%2 == 1 {
			name = strings.TrimPrefix(name, "$a")
			continue
		}
		s.add(symbol.Address, symbol.Size, name, core.BoundarySymbol, prioritySymbol)
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
		s.add(entry.Address, 12, entry.Name, core.BoundaryPLT, priorityPLT)
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

func (s *boundaryScanner) decode(address uint64) (armasm.Inst, bool) {
	region := s.regionFor(address)
	if region == nil {
		return armasm.Inst{}, false
	}
	data := s.regionData(region)
	offset := address - region.Address
	if offset+4 > uint64(len(data)) {
		return armasm.Inst{}, false
	}
	inst, err := armasm.Decode(data[offset:], armasm.ModeARM)
	if err != nil {
		return armasm.Inst{}, false
	}
	return inst, true
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
		inst, ok := s.decode(address)
		if !ok {
			continue
		}
		end := address + 4
		if end > limit {
			continue
		}
		if end > maxEnd {
			maxEnd = end
		}
		base := baseOp(inst.Op)
		switch base {
		case armasm.BL:
			if target, ok := staticTarget(inst, address); ok {
				calls = append(calls, target)
			}
			work = append(work, end)
		case armasm.B:
			if target, ok := staticTarget(inst, address); ok && target >= start && target < limit {
				work = append(work, target)
			}
		case armasm.BX, armasm.BKPT, armasm.SVC:
			// No successors inside this function.
		default:
			work = append(work, end)
		}
	}
	return maxEnd, calls
}

// staticTarget returns the branch target of one decoded instruction.
func staticTarget(inst armasm.Inst, address uint64) (uint64, bool) {
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		if offset, ok := pcRelative(arg); ok {
			return address + 8 + uint64(offset), true
		}
	}
	return 0, false
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

// addTailScan recovers functions that no metadata mentions. ARM instructions
// are fixed width, so every four-byte slot is a candidate; the scan trusts a
// slot only when the decoder accepts it and the instruction can start a
// function.
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

func (s *boundaryScanner) scanGap(start, end uint64) {
	for address := start; address+8 <= end; {
		inst, ok := s.decode(address)
		if !ok {
			address += 4
			continue
		}
		if baseOp(inst.Op) == armasm.NOP {
			address += 4
			continue
		}
		if !plausibleFunctionStart(inst) {
			address += 4
			continue
		}
		s.add(address, 0, "", core.BoundaryScan, priorityScan)
		address += 8
	}
}

// plausibleFunctionStart accepts the prologue forms gcc and clang emit: a
// stack pointer adjustment, a push, a frame move or a literal-pool load.
func plausibleFunctionStart(inst armasm.Inst) bool {
	switch baseOp(inst.Op) {
	case armasm.PUSH, armasm.SUB, armasm.MOV, armasm.LDR, armasm.B:
		return true
	default:
		return false
	}
}

// isDataWord reports whether a word cannot be an ARM instruction. The decoder
// rejects these, and the linker also leaves zero words between functions.
func isDataWord(raw uint32) bool {
	return raw&0xff000000 == 0
}
