package arm

import (
	"sort"

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
	// defaultMode is the encoding of the image entry point. It decides the mode
	// of every function that no symbol marks.
	defaultMode string
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
	// The entry point records the encoding of the code the compiler emitted
	// most of: an odd address means Thumb.
	scanner.defaultMode = "arm"
	if entry := image.Entry; entry != nil && *entry%2 == 1 {
		scanner.defaultMode = "thumb"
	}
	scanner.addSymbols()
	scanner.addEHFrame()
	scanner.addFunctionStarts()
	scanner.addInitArrays()
	scanner.addPLT()
	if entry := image.Entry; entry != nil {
		mode := "arm"
		if *entry%2 == 1 {
			mode = "thumb"
		}
		scanner.addWithMode(*entry, 0, "", core.BoundaryEntry, priorityEntry, mode)
	}
	scanner.markThumbFromSymbols()
	scanner.walkDirectCalls()
	scanner.walkThumbCalls()
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
	// An odd address marks Thumb code; the instruction stream itself starts at
	// the aligned address.
	address &^= 1
	if address == 0 || s.regionFor(address) == nil {
		return false
	}
	// ELF also carries marker symbols such as go:textfipsstart whose size is
	// one byte; a size smaller than one Thumb instruction is not a real range,
	// so let the next boundary decide it.
	if size < 2 {
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
		// ARM ELF marks Thumb code by setting the low bit of st_value, and the
		// mapping symbols $a, $t and $d mark the start of an ARM, Thumb or data
		// run. Both carry the mode in the low bit.
		mode := "arm"
		if symbol.Address%2 == 1 {
			mode = "thumb"
		}
		if symbol.Name == "$d" {
			// A data run is not a function.
			continue
		}
		s.addWithMode(symbol.Address, symbol.Size, symbol.Name, core.BoundarySymbol, prioritySymbol, mode)
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
		// The linker writes PLT stubs in ARM mode even when the rest of the
		// image is Thumb. The ARMv7 stub is three instructions: add ip, pc;
		// add ip, ip; ldr pc, [ip]. The literal lives in the GOT.
		s.addWithMode(entry.Address, 12, entry.Name, core.BoundaryPLT, priorityPLT, "arm")
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
			if s.insideExisting(target) {
				// A call into the middle of a known range is a decoding
				// artifact: a Thumb BLX reaches an ARM PLT stub through an odd
				// address, and rounding it produces an address two bytes into
				// the stub.
				continue
			}
			if s.add(target, 0, "", core.BoundaryCall, priorityCall) {
				queue = append(queue, target)
			}
		}
	}
}

// insideExisting reports whether one address falls inside a range another
// boundary already covers.
func (s *boundaryScanner) insideExisting(address uint64) bool {
	for start, entry := range s.entries {
		if entry.Size == 0 || address <= start {
			continue
		}
		if address < start+entry.Size {
			return true
		}
	}
	return false
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
		next := s.nextBoundary(address)
		// A range never crosses the next boundary. The recursive descent can
		// walk past a boundary it did not know about, and an overlapping range
		// would decode the same bytes twice.
		if next > address && (entry.Size == 0 || entry.Size > next-address) {
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

// scanGap recovers a function start that neither a symbol nor the call graph
// reached. The scan is conservative: a candidate must follow padding or an
// unconditional branch, because anything else is mid-function.
func (s *boundaryScanner) scanGap(start, end uint64) {
	// A Thumb image is mostly Thumb but carries ARM PLT stubs, so the scan step
	// follows the region, not the image default.
	step := uint64(4)
	if s.modeFor(start) == "thumb" {
		step = 2
	}
	padding := true
	for address := start; address+8 <= end; {
		inst, size, ok := s.decodeAny(address)
		if !ok {
			padding = true
			address += step
			continue
		}
		if baseOp(inst.Op) == armasm.NOP {
			address += uint64(size)
			continue
		}
		if padding && plausibleFunctionStart(inst) {
			s.addWithMode(address, 0, "", core.BoundaryScan, priorityScan, s.modeFor(address))
			padding = false
			address += uint64(size)
			continue
		}
		padding = baseOp(inst.Op) == armasm.B || baseOp(inst.Op) == armasm.BX
		address += uint64(size)
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
