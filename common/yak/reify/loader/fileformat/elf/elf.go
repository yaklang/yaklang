package elf

import (
	"context"
	"debug/elf"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// ELF loads Executable and Linkable Format images. OS, architecture and byte
// order are independent fields derived from the header.
type Loader struct{}

var (
	_ core.Loader = Loader{}
)

func (Loader) Kind() core.Format        { return core.FormatELF }
func (Loader) Name() string             { return "ELF" }
func (Loader) Description() string      { return core.FormatDescription(core.FormatELF) }
func (Loader) Match(prefix []byte) bool { return core.CheckFormat(prefix) == core.FormatELF }

func (Loader) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, _, diagnostics, err := inspectELF(r)
	if err != nil {
		return nil, err
	}
	return core.NewCheckResult(r, core.FormatELF, target, image, diagnostics)
}

func (Loader) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, hints, diagnostics, err := inspectELF(r)
	if err != nil {
		return nil, err
	}
	artifact, err := core.NewArtifact(ctx, r, core.FormatELF, target, image, diagnostics)
	if err != nil {
		return nil, err
	}
	artifact.Hints = hints
	return artifact, nil
}

func inspectELF(reader *core.ByteReader) (*core.Image, core.Target, *core.BoundaryHints, []core.Diagnostic, error) {
	file, err := elf.NewFile(reader)
	if err != nil {
		return nil, core.Target{}, nil, nil, err
	}
	defer file.Close()
	bits := 32
	if file.Class == elf.ELFCLASS64 {
		bits = 64
	}
	target := core.Target{
		OS:           elfOS(file.OSABI),
		Architecture: elfArchitecture(file.Machine, bits),
		Bits:         bits,
		ByteOrder:    core.ByteOrderOf(file.ByteOrder),
		ABI:          file.OSABI.String(),
		Machine:      file.Machine.String(),
	}
	if file.Machine == elf.EM_ARM {
		// ARM images carry both encodings. The entry point and the Thumb
		// function symbols set the low bit of the address, which is the only
		// per-address record of the mode.
		target.Mode = armMode(file)
	}
	image := &core.Image{ObjectType: file.Type.String(), Sections: make([]core.Section, 0, len(file.Sections))}
	// A relocatable object carries no assigned addresses; the sections are laid
	// out from this base so each range stays distinct.
	relocatableBase := uint64(0)
	if file.Entry != 0 {
		entry := file.Entry
		image.Entry = &entry
	}
	// sectionBases maps a section header index to the address the layout
	// assigned. The image skips SHT_NULL sections, so the indices differ.
	sectionBases := make(map[elf.SectionIndex]uint64, len(file.Sections))
	for index, section := range file.Sections {
		if section.Type == elf.SHT_NULL {
			continue
		}
		fileSize := section.FileSize
		if section.Type == elf.SHT_NOBITS {
			fileSize = 0
		}
		address := section.Addr
		if file.Type == elf.ET_REL && address == 0 {
			// A relocatable object has no assigned addresses: the linker places
			// each section. The loader lays the sections out in file order so
			// their ranges stay distinct, which is what the boundary scanner
			// needs to tell a code region from a data region.
			address = relocatableBase
			relocatableBase += (section.Size + 0xf) &^ 0xf
		}
		sectionBases[elf.SectionIndex(index)] = address
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Name,
			FileOffset:     section.Offset,
			FileSize:       fileSize,
			VirtualAddress: address,
			MemorySize:     section.Size,
			Flags:          uint64(section.Flags),
			Writable:       section.Flags&elf.SHF_WRITE != 0,
		})
	}
	// A relocatable symbol value is relative to its section, so the loader adds
	// the section base that the layout above assigned.
	relocations := relativeRelocations(file)
	descriptors := ppc64Descriptors(file, relocations)
	parseELFSymbols(file, image, sectionBases, file.Type == elf.ET_REL, descriptors)
	hints := parseELFHints(file, image, sectionBases)
	// A PowerPC ELFv1 call reaches the function descriptor; the symbol lookup needs
	// the code address it holds.
	hints.OpdDescriptors = descriptors
	image.Relocations = relocations
	return image, target, hints, nil, nil
}

type elfRelocation struct {
	offset   uint64
	symIndex uint32
	addend   uint64
}

// parseELFSymbols reads the static symbol table. When relocatable is true the
// symbol values are section-relative and need the section base.
// ppc64Descriptors maps each PowerPC ELFv1 function descriptor to the code
// address it holds. In that ABI a function symbol points at an eight-byte
// descriptor in .opd: the first word is the entry point and the second the TOC.
// Without the mapping a symbol reports an address in a data section, and the
// parser has nothing to decode.
func ppc64Descriptors(file *elf.File, relocations []core.Relocation) map[uint64]uint64 {
	if file.Machine != elf.EM_PPC64 || file.ByteOrder != binary.BigEndian {
		return nil
	}
	section := file.Section(".opd")
	if section == nil || section.Type == elf.SHT_NOBITS {
		return nil
	}
	data, err := section.Data()
	if err != nil {
		return nil
	}
	// A position-independent image leaves the descriptor empty in the file and
	// fills it through a relative relocation at load time.
	written := make(map[uint64]uint64, len(relocations))
	for _, relocation := range relocations {
		written[relocation.Address] = relocation.Value
	}
	descriptors := make(map[uint64]uint64, len(data)/8)
	for offset := 0; offset+8 <= len(data); offset += 8 {
		address := section.Addr + uint64(offset)
		entry := binary.BigEndian.Uint64(data[offset:])
		if entry == 0 {
			entry = written[address]
		}
		if entry == 0 {
			continue
		}
		descriptors[address] = entry
	}
	return descriptors
}

func parseELFSymbols(file *elf.File, image *core.Image, bases map[elf.SectionIndex]uint64, relocatable bool, descriptors map[uint64]uint64) {
	seen := make(map[string]bool)
	add := func(symbol sym.Symbol) {
		if symbol.Name == "" {
			return
		}
		key := fmt.Sprintf("%s@0x%x", symbol.Name, symbol.Address)
		if seen[key] {
			return
		}
		seen[key] = true
		image.Symbols = append(image.Symbols, symbol)
	}

	if symbols, err := file.Symbols(); err == nil {
		for _, symbol := range symbols {
			if symbol.Name == "" {
				continue
			}
			if symbol.Section == elf.SHN_UNDEF {
				add(sym.Symbol{Name: symbol.Name, Kind: elfSymbolKind(symbol), Imported: true, Library: symbol.Library})
				continue
			}
			if symbol.Value == 0 && !relocatable {
				// A zero value in a linked image names the section start, which
				// the symbol table also records as the section symbol.
				continue
			}
			address := symbol.Value
			if entry, ok := descriptors[address]; ok {
				// The symbol names a function descriptor; the code lives at the
				// address the descriptor holds.
				address = entry
			}
			if relocatable {
				// A relocatable value is relative to its section, and zero is a
				// legitimate offset: the first function of a section starts at
				// zero.
				address = bases[symbol.Section] + symbol.Value
			}
			add(sym.Symbol{
				Name:    symbol.Name,
				Address: address,
				Size:    symbol.Size,
				Kind:    elfSymbolKind(symbol),
				Library: symbol.Library,
			})
		}
	}

	dynamicSymbols, err := file.DynamicSymbols()
	if err != nil {
		return
	}
	for _, symbol := range dynamicSymbols {
		if symbol.Name == "" {
			continue
		}
		if symbol.Section == elf.SHN_UNDEF {
			add(sym.Symbol{Name: symbol.Name, Kind: elfSymbolKind(symbol), Imported: true, Library: symbol.Library})
			continue
		}
		if symbol.Value == 0 {
			continue
		}
		add(sym.Symbol{
			Name:    symbol.Name,
			Address: symbol.Value,
			Size:    symbol.Size,
			Kind:    elfSymbolKind(symbol),
			Library: symbol.Library,
		})
	}

	for _, set := range []struct {
		relocations []elfRelocation
		plt         bool
	}{
		{elfRelocations(file, ".rela.plt", ".rel.plt"), true},
		{elfRelocations(file, ".rela.dyn", ".rel.dyn"), false},
	} {
		for index, relocation := range set.relocations {
			if relocation.symIndex == 0 || int(relocation.symIndex) > len(dynamicSymbols) {
				continue
			}
			symbol := dynamicSymbols[relocation.symIndex-1]
			if symbol.Name == "" {
				continue
			}
			kind := elfSymbolKind(symbol)
			add(sym.Symbol{
				Name:     symbol.Name,
				Address:  relocation.offset,
				Size:     8,
				Kind:     kind,
				Imported: true,
				Library:  symbol.Library,
			})
			if !set.plt {
				continue
			}
			pltAddress, ok := elfPLTAddress(file, index)
			if !ok {
				continue
			}
			add(sym.Symbol{
				Name:     symbol.Name,
				Address:  pltAddress,
				Size:     16,
				Kind:     sym.SymbolFunction,
				Imported: true,
				Library:  symbol.Library,
			})
		}
	}
}

func elfRelocations(file *elf.File, relaName, relName string) []elfRelocation {
	section := file.Section(relaName)
	hasAddend := true
	if section == nil {
		section = file.Section(relName)
		hasAddend = false
	}
	if section == nil {
		return nil
	}
	data, err := section.Data()
	if err != nil {
		return nil
	}
	var entrySize uint64
	switch file.Class {
	case elf.ELFCLASS64:
		if hasAddend {
			entrySize = 24
		} else {
			entrySize = 16
		}
	case elf.ELFCLASS32:
		if hasAddend {
			entrySize = 12
		} else {
			entrySize = 8
		}
	default:
		return nil
	}
	var result []elfRelocation
	for offset := uint64(0); offset+entrySize <= uint64(len(data)); offset += entrySize {
		switch file.Class {
		case elf.ELFCLASS64:
			entry := elfRelocation{
				offset:   file.ByteOrder.Uint64(data[offset:]),
				symIndex: elf.R_SYM64(file.ByteOrder.Uint64(data[offset+8:])),
			}
			if hasAddend {
				entry.addend = file.ByteOrder.Uint64(data[offset+16:])
			}
			result = append(result, entry)
		case elf.ELFCLASS32:
			entry := elfRelocation{
				offset:   uint64(file.ByteOrder.Uint32(data[offset:])),
				symIndex: elf.R_SYM32(file.ByteOrder.Uint32(data[offset+4:])),
			}
			if hasAddend {
				entry.addend = uint64(file.ByteOrder.Uint32(data[offset+8:]))
			}
			result = append(result, entry)
		}
	}
	return result
}

// elfPLTAddress maps a relocation index to its PLT stub. x86-64 reserves the
// first .plt entry; .plt.sec, when present, has one entry per import.
func elfPLTAddress(file *elf.File, index int) (uint64, bool) {
	// The stub size follows the relocation the architecture uses: sixteen bytes
	// on x86, ARM64, RISC-V and LoongArch; twenty-four on PowerPC64; thirty-two
	// on s390x; twelve on 32-bit ARM, which keeps a four-byte literal.
	stride := uint64(16)
	switch file.Machine {
	case elf.EM_386, elf.EM_X86_64, elf.EM_AARCH64, elf.EM_RISCV, elf.EM_LOONGARCH, elf.EM_ARM:
	case elf.EM_PPC64:
		stride = 24
	case elf.EM_S390:
		stride = 32
	default:
		// The remaining architectures place their import stubs elsewhere or use a
		// layout that this formula does not describe.
		return 0, false
	}
	if file.Machine == elf.EM_ARM {
		// The ARM PLT is not uniform: the linker inserts a four-byte Thumb
		// trampoline where an import is only reachable through one. Reading the
		// stub addresses out of the section is exact, a fixed stride is not.
		if entries := armPLTEntries(file); entries != nil {
			if index < len(entries) {
				return entries[index], true
			}
			return 0, false
		}
		stride = 12
	}
	if section := file.Section(".plt.sec"); section != nil && section.Addr != 0 {
		return section.Addr + uint64(index)*stride, true
	}
	if section := file.Section(".plt"); section != nil && section.Addr != 0 {
		offset, ok := pltStubOffset(file.Machine, index)
		if !ok {
			return 0, false
		}
		return section.Addr + offset, true
	}
	return 0, false
}

// pltStubOffset returns the distance from the start of .plt to the stub of one
// relocation. The first entry follows the resolver header, whose size differs by
// architecture: one stub on x86 and PowerPC64, two on ARM, ARM64, RISC-V and
// LoongArch, and a single wide entry on s390x.
func pltStubOffset(machine elf.Machine, index int) (uint64, bool) {
	stride := uint64(16)
	header := uint64(1)
	switch machine {
	case elf.EM_386, elf.EM_X86_64:
	case elf.EM_ARM, elf.EM_AARCH64, elf.EM_RISCV, elf.EM_LOONGARCH:
		header = 2
	case elf.EM_PPC64:
		stride = 24
	case elf.EM_S390:
		stride = 32
	default:
		// The remaining architectures place their import stubs elsewhere or use a
		// layout that this formula does not describe.
		return 0, false
	}
	return (uint64(index) + header) * stride, true
}

// armPLTEntries returns the address of every ARM PLT stub in relocation order.
// The relocation order matches the section order.
func armPLTEntries(file *elf.File) []uint64 {
	section := file.Section(".plt")
	if section == nil || section.Addr == 0 {
		return nil
	}
	data, err := section.Data()
	if err != nil {
		return nil
	}
	offsets := armPLTStubOffsets(data)
	entries := make([]uint64, 0, len(offsets))
	for _, offset := range offsets {
		entries = append(entries, section.Addr+offset)
	}
	return entries
}

// armPLTStubOffsets returns the byte offset of every ARM PLT stub in one .plt
// payload. A stub is three instructions: add ip, pc, #0; add ip, ip, #imm;
// ldr pc, [ip, #imm]. The immediate in the second instruction is the distance to
// the GOT, so the match ignores the immediate fields.
func armPLTStubOffsets(data []byte) []uint64 {
	const (
		addIPPC    = 0xe28fc000 // add ip, pc, #imm
		addIPIP    = 0xe28cc000 // add ip, ip, #imm
		loadPCPop  = 0xe5bcf000 // ldr pc, [ip, #imm]
		opcodeMask = 0xfffff000
	)
	var offsets []uint64
	for offset := 0; offset+12 <= len(data); offset += 4 {
		if binary.LittleEndian.Uint32(data[offset:])&opcodeMask != addIPPC {
			continue
		}
		if binary.LittleEndian.Uint32(data[offset+4:])&opcodeMask != addIPIP {
			continue
		}
		if binary.LittleEndian.Uint32(data[offset+8:])&opcodeMask != loadPCPop {
			continue
		}
		offsets = append(offsets, uint64(offset))
	}
	return offsets
}

func elfSymbolKind(symbol elf.Symbol) sym.SymbolKind {
	switch elf.ST_TYPE(symbol.Info) {
	case elf.STT_FUNC:
		return sym.SymbolFunction
	case elf.STT_OBJECT:
		return sym.SymbolObject
	default:
		return sym.SymbolUnknown
	}
}

// armMode reports the mode of the image entry point. A set low bit means Thumb.
// Images whose entry is ARM still carry Thumb functions, so the per-function
// boundary records the mode it discovered.
func armMode(file *elf.File) string {
	if file.Entry%2 == 1 {
		return "thumb"
	}
	return "arm"
}

func elfArchitecture(machine elf.Machine, bits int) core.Architecture {
	switch machine {
	case elf.EM_X86_64:
		return core.ArchAMD64
	case elf.EM_386:
		return core.Arch386
	case elf.EM_AARCH64:
		return core.ArchARM64
	case elf.EM_ARM:
		return core.ArchARM
	case elf.EM_RISCV:
		if bits == 64 {
			return core.ArchRISCV64
		}
		return core.ArchRISCV32
	case elf.EM_MIPS:
		if bits == 64 {
			return core.ArchMIPS64
		}
		return core.ArchMIPS
	case elf.EM_PPC:
		return core.ArchPPC
	case elf.EM_PPC64:
		return core.ArchPPC64
	case elf.EM_S390:
		return core.ArchS390X
	case elf.EM_LOONGARCH:
		return core.ArchLOONG64
	default:
		return core.ArchUnknown
	}
}

func elfOS(abi elf.OSABI) core.OperatingSystem {
	switch abi {
	case elf.ELFOSABI_LINUX:
		return core.OSLinux
	case elf.ELFOSABI_FREEBSD:
		return core.OSFreeBSD
	case elf.ELFOSABI_NETBSD:
		return core.OSNetBSD
	case elf.ELFOSABI_OPENBSD:
		return core.OSOpenBSD
	case elf.ELFOSABI_SOLARIS:
		return core.OSSolaris
	default:
		// System V, HP-UX, Standalone and unknown ABIs do not identify an OS.
		return core.OSUnknown
	}
}

// ELFArchitecture exposes the machine mapping for tests and callers.
func ELFArchitecture(machine elf.Machine, bits int) core.Architecture {
	return elfArchitecture(machine, bits)
}

// LoaderArchitecture is the exported spelling of the machine mapping; callers
// outside the package use this one.
func LoaderArchitecture(machine elf.Machine, bits int) core.Architecture {
	return elfArchitecture(machine, bits)
}

// parseELFHints collects the format facts an architecture boundary discoverer
// needs: executable ranges, .eh_frame, init/fini targets, PLT stubs and GOT
// resolutions. It never decodes instructions.
func parseELFHints(file *elf.File, image *core.Image, bases map[elf.SectionIndex]uint64) *core.BoundaryHints {
	hints := &core.BoundaryHints{GOTRelocs: make(map[uint64]uint64)}
	if file.Machine == elf.EM_PPC64 && file.ByteOrder == binary.LittleEndian {
		// ELFv2 keeps a local entry point eight bytes past the global one, and a
		// local call targets it.
		hints.LocalEntryOffset = 8
	}
	for index, section := range file.Sections {
		if section.Flags&elf.SHF_EXECINSTR == 0 || section.FileSize == 0 {
			continue
		}
		// A relocatable object records no addresses, so the region address has
		// to be the one the layout assigned. Using the raw header value would
		// put every executable section at zero and a lookup by address would
		// then find the wrong section.
		address := section.Addr
		if base, ok := bases[elf.SectionIndex(index)]; ok && address == 0 {
			address = base
		}
		hints.ExecSections = append(hints.ExecSections, core.Region{
			Address: address,
			Offset:  section.Offset,
			Size:    section.FileSize,
		})
		// The ARM linker emits the PLT and the init stub in ARM mode even when
		// the rest of the image is Thumb. Record that so the boundary scanner
		// decodes each range with the right decoder.
		if file.Machine == elf.EM_ARM && (section.Name == ".plt" || section.Name == ".init" || section.Name == ".plt.sec") {
			hints.ModeRegions = append(hints.ModeRegions, core.ModeRegion{
				Region: core.Region{Address: address, Offset: section.Offset, Size: section.FileSize},
				Mode:   "arm",
			})
		}
	}
	if section := file.Section(".eh_frame"); section != nil && section.FileSize > 0 {
		if data, err := section.Data(); err == nil {
			hints.EHFrame = data
			hints.EHFrameAddr = section.Addr
		}
	}

	relocTargets := elfRelocationTargets(file)
	for _, name := range []string{".init_array", ".fini_array"} {
		section := file.Section(name)
		if section == nil || section.FileSize < 8 {
			continue
		}
		data, err := section.Data()
		if err != nil {
			continue
		}
		for offset := uint64(0); offset+8 <= uint64(len(data)); offset += 8 {
			address := file.ByteOrder.Uint64(data[offset:])
			if target, ok := relocTargets[section.Addr+offset]; ok {
				address = target
			}
			if address != 0 {
				hints.InitTargets = append(hints.InitTargets, address)
			}
		}
	}

	dynamicSymbols, _ := file.DynamicSymbols()
	for _, set := range []struct {
		relocations []elfRelocation
		plt         bool
	}{
		{elfRelocations(file, ".rela.plt", ".rel.plt"), true},
		{elfRelocations(file, ".rela.dyn", ".rel.dyn"), false},
	} {
		for index, relocation := range set.relocations {
			if relocation.symIndex == 0 || int(relocation.symIndex) > len(dynamicSymbols) {
				continue
			}
			symbol := dynamicSymbols[relocation.symIndex-1]
			if symbol.Name == "" {
				continue
			}
			if set.plt {
				if pltAddress, ok := elfPLTAddress(file, index); ok {
					hints.PLTTargets = append(hints.PLTTargets, core.PLTEntry{Address: pltAddress, Name: symbol.Name})
					hints.GOTRelocs[relocation.offset] = pltAddress
				}
				continue
			}
			if target, ok := relocTargets[relocation.offset]; ok {
				hints.GOTRelocs[relocation.offset] = target
			}
		}
	}
	return hints
}

// elfRelocationTargets maps a relocated slot to its resolved image address.
func elfRelocationTargets(file *elf.File) map[uint64]uint64 {
	result := make(map[uint64]uint64)
	dynamicSymbols, _ := file.DynamicSymbols()
	for _, name := range []string{".rela.dyn", ".rela.plt"} {
		section := file.Section(name)
		if section == nil {
			continue
		}
		data, err := section.Data()
		if err != nil {
			continue
		}
		const entrySize = 24
		for offset := 0; offset+entrySize <= len(data); offset += entrySize {
			slot := file.ByteOrder.Uint64(data[offset:])
			info := file.ByteOrder.Uint64(data[offset+8:])
			addend := int64(file.ByteOrder.Uint64(data[offset+16:]))
			symbolIndex := int(elf.R_SYM64(info))
			target := addend
			if symbolIndex > 0 && symbolIndex <= len(dynamicSymbols) {
				symbol := dynamicSymbols[symbolIndex-1]
				if symbol.Section != elf.SHN_UNDEF {
					target += int64(symbol.Value)
				}
			}
			if target > 0 {
				result[slot] = uint64(target)
			}
		}
	}
	return result
}

// relativeRelocations lists the value the dynamic linker writes at an address. A
// relative relocation carries no symbol: the addend is the address to write. A
// position-independent image leaves zero in the file, so a reader that ignores
// these entries sees an empty slot where a pointer is.
func relativeRelocations(file *elf.File) []core.Relocation {
	var result []core.Relocation
	for _, relocation := range elfRelocations(file, ".rela.dyn", ".rel.dyn") {
		if relocation.symIndex != 0 {
			continue
		}
		result = append(result, core.Relocation{Address: relocation.offset, Value: relocation.addend})
	}
	return result
}
