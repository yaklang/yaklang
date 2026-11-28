package x86

import (
	"sort"

	"golang.org/x/arch/x86/x86asm"

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

type boundary struct {
	core.FunctionBoundary
	priority boundaryPriority
}

type boundaryScanner struct {
	profile     Profile
	reader      *core.ByteReader
	hints       core.BoundaryHints
	image       *core.Image
	entries     map[uint64]*boundary
	sortedAddrs []uint64
	pending     []uint64
	chunkRegion uint64
	chunkStart  uint64
	chunkLen    uint64
	chunkBuf    []byte
	stats       core.BoundaryStats
}

const scannerChunkSize = 1 << 20

// discoverBoundaries finds function starts using loader hints, symbols, entry,
// direct calls and tail scanning. It does not build LowIR; asm2lowir uses the
// result to call Parse per function.
func discoverBoundaries(profile Profile, reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	if reader == nil || image == nil || len(hints.ExecSections) == 0 {
		return core.BoundaryResult{}, nil
	}
	scanner := &boundaryScanner{
		profile: profile,
		reader:  reader,
		hints:   hints,
		image:   image,
		entries: make(map[uint64]*boundary),
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
	// Release temporary scan buffer before returning.
	scanner.chunkBuf = nil
	scanner.sortedAddrs = nil
	scanner.pending = nil

	functions := make([]core.FunctionBoundary, 0, len(scanner.entries))
	for _, boundary := range scanner.entries {
		functions = append(functions, boundary.FunctionBoundary)
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i].Address < functions[j].Address })
	return core.BoundaryResult{Functions: functions, Stats: scanner.stats}, nil
}

func (s *boundaryScanner) add(address, size uint64, name string, source core.BoundarySource, priority boundaryPriority) bool {
	if s.regionFor(address) == nil {
		return false
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
	s.entries[address] = &boundary{
		FunctionBoundary: core.FunctionBoundary{Address: address, Size: size, Name: name, Source: source},
		priority:         priority,
	}
	s.pending = append(s.pending, address)
	return true
}

func (s *boundaryScanner) flushAddrs() {
	if len(s.pending) == 0 {
		return
	}
	s.sortedAddrs = append(s.sortedAddrs, s.pending...)
	s.pending = s.pending[:0]
	sort.Slice(s.sortedAddrs, func(i, j int) bool { return s.sortedAddrs[i] < s.sortedAddrs[j] })
}

func (s *boundaryScanner) addSymbols() {
	for _, symbol := range s.image.Symbols {
		if symbol.Name == "" || symbol.Kind != sym.SymbolFunction || symbol.Imported {
			continue
		}
		s.add(symbol.Address, symbol.Size, symbol.Name, core.BoundarySymbol, prioritySymbol)
	}
}

func (s *boundaryScanner) addPLT() {
	for _, entry := range s.hints.PLTTargets {
		s.add(entry.Address, 16, entry.Name, core.BoundaryPLT, priorityPLT)
	}
}

func (s *boundaryScanner) addInitArrays() {
	for _, address := range s.hints.InitTargets {
		s.add(address, 0, "", core.BoundaryInitArray, priorityInitArray)
	}
}

// addFunctionStarts inserts exact entries from format metadata: Mach-O
// LC_FUNCTION_STARTS, PE export tables and PE .pdata.
func (s *boundaryScanner) addFunctionStarts() {
	for _, address := range s.hints.FunctionStarts {
		s.add(address, 0, "", core.BoundaryFunctionStart, priorityFunctionStart)
	}
}

// addEHFrame inserts one boundary per FDE range. CFI is not executed.
func (s *boundaryScanner) addEHFrame() {
	for _, entry := range ehframe.Entries(s.hints.EHFrame, s.hints.EHFrameAddr) {
		s.add(entry.Location, entry.Range, "", core.BoundaryEHFrame, priorityEHFrame)
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

func (s *boundaryScanner) readBytesAt(region *core.Region, address uint64, need uint64) []byte {
	if region == nil || address < region.Address {
		return nil
	}
	offset := address - region.Address
	if offset >= region.Size {
		return nil
	}
	avail := region.Size - offset
	if need > avail {
		need = avail
	}
	if need == 0 {
		return nil
	}
	if s.chunkRegion == region.Address && s.chunkLen > 0 &&
		address >= s.chunkStart && address-s.chunkStart+need <= s.chunkLen {
		start := int(address - s.chunkStart)
		end := start + int(avail)
		if end > int(s.chunkLen) {
			end = int(s.chunkLen)
		}
		return s.chunkBuf[start:end]
	}
	chunkSize := uint64(scannerChunkSize)
	if chunkSize < need {
		chunkSize = need
	}
	if avail < chunkSize {
		chunkSize = avail
	}
	if uint64(cap(s.chunkBuf)) < chunkSize {
		s.chunkBuf = make([]byte, int(chunkSize))
	}
	n, err := s.reader.ReadAt(s.chunkBuf[:int(chunkSize)], int64(region.Offset+offset))
	if n == 0 || (err != nil && n < int(need)) {
		s.chunkLen = 0
		return nil
	}
	s.chunkRegion = region.Address
	s.chunkStart = address
	s.chunkLen = uint64(n)
	return s.chunkBuf[:n]
}

func (s *boundaryScanner) decode(address uint64) (x86asm.Inst, bool) {
	region := s.regionFor(address)
	if region == nil {
		return x86asm.Inst{}, false
	}
	data := s.readBytesAt(region, address, 15)
	if len(data) == 0 {
		return x86asm.Inst{}, false
	}
	if len(data) >= 4 && data[0] == 0xf3 && data[1] == 0x0f && data[2] == 0x1e && (data[3] == 0xfa || data[3] == 0xfb) {
		return x86asm.Inst{Op: x86asm.NOP, Len: 4}, true
	}
	inst, err := x86asm.Decode(data, s.profile.Mode)
	if err != nil || inst.Len <= 0 {
		return x86asm.Inst{}, false
	}
	return inst, true
}

func (s *boundaryScanner) nextBoundary(address uint64) uint64 {
	if len(s.pending) > 64 {
		s.flushAddrs()
	}
	next := uint64(0)
	if pos := sort.Search(len(s.sortedAddrs), func(i int) bool { return s.sortedAddrs[i] > address }); pos < len(s.sortedAddrs) {
		next = s.sortedAddrs[pos]
	}
	for _, candidate := range s.pending {
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

func (s *boundaryScanner) indirectGOTTarget(inst x86asm.Inst, address uint64) (uint64, bool) {
	if len(inst.Args) == 0 {
		return 0, false
	}
	mem, ok := inst.Args[0].(x86asm.Mem)
	if !ok || s.profile.InstructionPointer == "" || mem.Base != x86asm.RIP {
		return 0, false
	}
	got := address + uint64(inst.Len) + uint64(int64(mem.Disp))
	target, ok := s.hints.GOTRelocs[got]
	return target, ok && target != 0
}

func (s *boundaryScanner) finishRanges() {
	s.flushAddrs()
	for idx, address := range s.sortedAddrs {
		entry := s.entries[address]
		if entry == nil || entry.Size != 0 {
			continue
		}
		var next uint64
		if idx+1 < len(s.sortedAddrs) {
			next = s.sortedAddrs[idx+1]
		}
		if region := s.regionFor(address); region != nil {
			end := region.Address + region.Size
			if next == 0 || next > end {
				next = end
			}
		}
		if next > address {
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

func (s *boundaryScanner) scanGap(start, end uint64) {
	for address := start; address+16 <= end; {
		region := s.regionFor(address)
		if region == nil {
			return
		}
		data := s.readBytesAt(region, address, 1)
		if len(data) == 0 {
			return
		}
		if data[0] == 0xcc || data[0] == 0x90 || data[0] == 0x00 {
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
		s.add(address, 0, "", core.BoundaryScan, priorityScan)
		address += 16
	}
}
