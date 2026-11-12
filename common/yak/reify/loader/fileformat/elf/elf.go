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
		})
	}
	// A relocatable symbol value is relative to its section, so the loader adds
	// the section base that the layout above assigned.
	parseELFSymbols(file, image, sectionBases, file.Type == elf.ET_REL)
	return image, target, parseELFHints(file, image), nil, nil
}

type elfRelocation struct {
	offset   uint64
	symIndex uint32
}

// parseELFSymbols reads the static symbol table. When relocatable is true the
// symbol values are section-relative and need the section base.
func parseELFSymbols(file *elf.File, image *core.Image, bases map[elf.SectionIndex]uint64, relocatable bool) {
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
			if !set.plt || file.Machine != elf.EM_X86_64 {
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
			result = append(result, elfRelocation{
				offset:   file.ByteOrder.Uint64(data[offset:]),
				symIndex: elf.R_SYM64(file.ByteOrder.Uint64(data[offset+8:])),
			})
		case elf.ELFCLASS32:
			result = append(result, elfRelocation{
				offset:   uint64(file.ByteOrder.Uint32(data[offset:])),
				symIndex: elf.R_SYM32(file.ByteOrder.Uint32(data[offset+4:])),
			})
		}
	}
	return result
}

// elfPLTAddress maps a relocation index to its PLT stub. x86-64 reserves the
// first .plt entry; .plt.sec, when present, has one entry per import.
func elfPLTAddress(file *elf.File, index int) (uint64, bool) {
	// The stub size follows the relocation the architecture uses: sixteen bytes
	// on x86 and ARM64, twelve on 32-bit ARM, which keeps a four-byte literal.
	stride := uint64(16)
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
		header := uint64(1)
		if file.Machine == elf.EM_ARM {
			// The ARM PLT header occupies two stub slots on ARMv7.
			header = 2
		}
		return section.Addr + (uint64(index)+header)*stride, true
	}
	return 0, false
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
func parseELFHints(file *elf.File, image *core.Image) *core.BoundaryHints {
	hints := &core.BoundaryHints{GOTRelocs: make(map[uint64]uint64)}
	for _, section := range file.Sections {
		if section.Flags&elf.SHF_EXECINSTR == 0 || section.FileSize == 0 {
			continue
		}
		hints.ExecSections = append(hints.ExecSections, core.Region{
			Address: section.Addr,
			Offset:  section.Offset,
			Size:    section.FileSize,
		})
		// The ARM linker emits the PLT and the init stub in ARM mode even when
		// the rest of the image is Thumb. Record that so the boundary scanner
		// decodes each range with the right decoder.
		if file.Machine == elf.EM_ARM && (section.Name == ".plt" || section.Name == ".init" || section.Name == ".plt.sec") {
			hints.ModeRegions = append(hints.ModeRegions, core.ModeRegion{
				Region: core.Region{Address: section.Addr, Offset: section.Offset, Size: section.FileSize},
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
