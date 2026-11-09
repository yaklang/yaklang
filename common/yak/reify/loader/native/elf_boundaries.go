package native

import (
	"debug/elf"
	"sort"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type elfBoundaryPriority int

const (
	prioritySymbol elfBoundaryPriority = iota
	priorityEHFrame
	priorityInitArray
	priorityPLT
	priorityEntry
	priorityCall
	priorityScan
)

type elfBoundary struct {
	core.FunctionBoundary
	priority elfBoundaryPriority
}

type elfBoundaryScanner struct {
	file       *elf.File
	image      *core.Image
	entries    map[uint64]*elfBoundary
	exec       []*elf.Section
	data       map[string][]byte
	relocs     map[uint64]uint64
	stats      core.BoundaryStats
	sectionEnd map[string]uint64
}

func discoverELFunctions(file *elf.File, image *core.Image) {
	if file == nil || image == nil || file.Machine != elf.EM_X86_64 {
		return
	}
	scanner := &elfBoundaryScanner{
		file:       file,
		image:      image,
		entries:    make(map[uint64]*elfBoundary),
		data:       make(map[string][]byte),
		relocs:     make(map[uint64]uint64),
		sectionEnd: make(map[string]uint64),
	}
	for _, section := range file.Sections {
		if section.Flags&elf.SHF_EXECINSTR == 0 || section.FileSize == 0 || section.Size == 0 {
			continue
		}
		scanner.exec = append(scanner.exec, section)
	}
	if len(scanner.exec) == 0 {
		return
	}
	scanner.readRelocations()
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
	image.Functions = functions
	image.BoundaryStats = scanner.stats
}

func (s *elfBoundaryScanner) add(address, size uint64, name string, source core.BoundarySource, priority elfBoundaryPriority) bool {
	if address == 0 || s.sectionFor(address) == nil {
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
	s.entries[address] = &elfBoundary{
		FunctionBoundary: core.FunctionBoundary{Address: address, Size: size, Name: name, Source: source},
		priority:         priority,
	}
	return true
}

func (s *elfBoundaryScanner) addSymbols() {
	if symbols, err := s.file.Symbols(); err == nil {
		for _, symbol := range symbols {
			if symbol.Name == "" || elf.ST_TYPE(symbol.Info) != elf.STT_FUNC {
				continue
			}
			if symbol.Section == elf.SHN_UNDEF || symbol.Value == 0 {
				continue
			}
			s.add(symbol.Value, symbol.Size, symbol.Name, core.BoundarySymbol, prioritySymbol)
		}
	}
	if symbols, err := s.file.DynamicSymbols(); err == nil {
		for _, symbol := range symbols {
			if symbol.Name == "" || elf.ST_TYPE(symbol.Info) != elf.STT_FUNC {
				continue
			}
			if symbol.Section == elf.SHN_UNDEF || symbol.Value == 0 {
				continue
			}
			s.add(symbol.Value, symbol.Size, symbol.Name, core.BoundarySymbol, prioritySymbol)
		}
	}
}

func (s *elfBoundaryScanner) addPLT() {
	for _, symbol := range s.image.Symbols {
		if !symbol.Imported || symbol.Kind != core.SymbolFunction || symbol.Address == 0 {
			continue
		}
		section := s.sectionFor(symbol.Address)
		if section == nil || (section.Name != ".plt" && section.Name != ".plt.sec") {
			continue
		}
		s.add(symbol.Address, 16, symbol.Name, core.BoundaryPLT, priorityPLT)
	}
}

func (s *elfBoundaryScanner) addInitArrays() {
	for _, name := range []string{".init_array", ".fini_array"} {
		section := s.file.Section(name)
		if section == nil || section.FileSize < 8 {
			continue
		}
		data, err := section.Data()
		if err != nil {
			continue
		}
		for offset := uint64(0); offset+8 <= uint64(len(data)); offset += 8 {
			address := s.file.ByteOrder.Uint64(data[offset:])
			if target, ok := s.relocs[section.Addr+offset]; ok {
				address = target
			}
			s.add(address, 0, "", core.BoundaryInitArray, priorityInitArray)
		}
	}
}

func (s *elfBoundaryScanner) readRelocations() {
	for _, name := range []string{".rela.dyn", ".rela.plt"} {
		section := s.file.Section(name)
		if section == nil {
			continue
		}
		data, err := section.Data()
		if err != nil {
			continue
		}
		const entrySize = 24
		for offset := 0; offset+entrySize <= len(data); offset += entrySize {
			relocationOffset := s.file.ByteOrder.Uint64(data[offset:])
			info := s.file.ByteOrder.Uint64(data[offset+8:])
			addend := int64(s.file.ByteOrder.Uint64(data[offset+16:]))
			symbolIndex := int(elf.R_SYM64(info))
			target := addend
			if symbolIndex > 0 {
				if symbols, err := s.file.DynamicSymbols(); err == nil && symbolIndex <= len(symbols) {
					symbol := symbols[symbolIndex-1]
					if symbol.Section != elf.SHN_UNDEF {
						target += int64(symbol.Value)
					}
				}
			}
			if target > 0 {
				s.relocs[relocationOffset] = uint64(target)
			}
		}
	}
}

func (s *elfBoundaryScanner) sectionFor(address uint64) *elf.Section {
	for _, section := range s.exec {
		if address >= section.Addr && address-section.Addr < section.FileSize {
			return section
		}
	}
	return nil
}

func (s *elfBoundaryScanner) sectionData(section *elf.Section) []byte {
	if data, ok := s.data[section.Name]; ok {
		return data
	}
	data, _ := section.Data()
	s.data[section.Name] = data
	return data
}

func (s *elfBoundaryScanner) decode(address uint64) (x86asm.Inst, bool) {
	section := s.sectionFor(address)
	if section == nil {
		return x86asm.Inst{}, false
	}
	data := s.sectionData(section)
	offset := address - section.Addr
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

func (s *elfBoundaryScanner) nextBoundary(address uint64) uint64 {
	next := uint64(0)
	for candidate := range s.entries {
		if candidate > address && (next == 0 || candidate < next) {
			next = candidate
		}
	}
	if section := s.sectionFor(address); section != nil {
		end := section.Addr + section.FileSize
		if next == 0 || next > end {
			return end
		}
	}
	return next
}

func (s *elfBoundaryScanner) walkDirectCalls() {
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
		if s.sectionFor(start) == nil {
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

func (s *elfBoundaryScanner) walkFunction(start, limit uint64) (uint64, []uint64) {
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
				// Tail jump to another function ends this function.
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

func (s *elfBoundaryScanner) indirectGOTTarget(inst x86asm.Inst, address uint64) (uint64, bool) {
	if len(inst.Args) == 0 {
		return 0, false
	}
	mem, ok := inst.Args[0].(x86asm.Mem)
	if !ok || mem.Base != x86asm.RIP {
		return 0, false
	}
	got := address + uint64(inst.Len) + uint64(int64(mem.Disp))
	for _, symbol := range s.image.Symbols {
		if symbol.Address != got || !symbol.Imported || symbol.Name == "" {
			continue
		}
		for _, candidate := range s.image.Symbols {
			if candidate.Name == symbol.Name && candidate.Kind == core.SymbolFunction && candidate.Address != 0 && candidate.Address != got {
				return candidate.Address, true
			}
		}
	}
	return 0, false
}

func isConditionalJump(op x86asm.Op) bool {
	switch op {
	case x86asm.JA, x86asm.JAE, x86asm.JB, x86asm.JBE, x86asm.JCXZ, x86asm.JECXZ, x86asm.JG, x86asm.JGE, x86asm.JL, x86asm.JLE, x86asm.JNE, x86asm.JNO, x86asm.JNP, x86asm.JNS, x86asm.JO, x86asm.JP, x86asm.JRCXZ, x86asm.JS:
		return true
	default:
		return false
	}
}

func (s *elfBoundaryScanner) finishRanges() {
	addresses := make([]uint64, 0, len(s.entries))
	for address := range s.entries {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
	for index, address := range addresses {
		entry := s.entries[address]
		if entry.Size != 0 {
			continue
		}
		next := s.nextBoundary(address)
		if next > address {
			entry.Size = next - address
		}
		_ = index
	}
}

func (s *elfBoundaryScanner) addTailScan() {
	covered := make([]core.FunctionBoundary, 0, len(s.entries))
	for _, entry := range s.entries {
		covered = append(covered, entry.FunctionBoundary)
	}
	sort.Slice(covered, func(i, j int) bool { return covered[i].Address < covered[j].Address })
	for _, section := range s.exec {
		start := section.Addr
		end := section.Addr + section.FileSize
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

func (s *elfBoundaryScanner) scanGap(start, end uint64) {
	for address := start; address+16 <= end; {
		data := s.sectionData(s.sectionFor(address))
		offset := address - s.sectionFor(address).Addr
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
