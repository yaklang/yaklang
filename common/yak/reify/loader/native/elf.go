package native

import (
	"context"
	"debug/elf"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// ELF loads Executable and Linkable Format images. OS, architecture and byte
// order are independent fields derived from the header.
type ELF struct{}

var (
	_ core.Loader = ELF{}
)

func (ELF) Kind() core.Format        { return core.FormatELF }
func (ELF) Name() string             { return "ELF" }
func (ELF) Description() string      { return describe(core.FormatELF) }
func (ELF) Match(prefix []byte) bool { return matchFormat(prefix, core.FormatELF) }

func (ELF) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, diagnostics, err := inspectELF(r)
	if err != nil {
		return nil, err
	}
	return checkResult(r, core.FormatELF, target, image, diagnostics)
}

func (ELF) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, diagnostics, err := inspectELF(r)
	if err != nil {
		return nil, err
	}
	return newArtifact(ctx, r, core.FormatELF, target, image, diagnostics)
}

func inspectELF(reader *core.ByteReader) (*core.Image, core.Target, []core.Diagnostic, error) {
	file, err := elf.NewFile(reader)
	if err != nil {
		return nil, core.Target{}, nil, err
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
	image := &core.Image{ObjectType: file.Type.String(), Sections: make([]core.Section, 0, len(file.Sections))}
	if file.Entry != 0 {
		entry := file.Entry
		image.Entry = &entry
	}
	for _, section := range file.Sections {
		if section.Type == elf.SHT_NULL {
			continue
		}
		fileSize := section.FileSize
		if section.Type == elf.SHT_NOBITS {
			fileSize = 0
		}
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Name,
			FileOffset:     section.Offset,
			FileSize:       fileSize,
			VirtualAddress: section.Addr,
			MemorySize:     section.Size,
			Flags:          uint64(section.Flags),
		})
	}
	parseELFSymbols(file, image)
	discoverELFunctions(file, image)
	return image, target, nil, nil
}

type elfRelocation struct {
	offset   uint64
	symIndex uint32
}

func parseELFSymbols(file *elf.File, image *core.Image) {
	seen := make(map[string]bool)
	add := func(symbol core.Symbol) {
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
				add(core.Symbol{Name: symbol.Name, Kind: elfSymbolKind(symbol), Imported: true, Library: symbol.Library})
				continue
			}
			if symbol.Value == 0 {
				continue
			}
			add(core.Symbol{
				Name:    symbol.Name,
				Address: symbol.Value,
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
			add(core.Symbol{Name: symbol.Name, Kind: elfSymbolKind(symbol), Imported: true, Library: symbol.Library})
			continue
		}
		if symbol.Value == 0 {
			continue
		}
		add(core.Symbol{
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
			add(core.Symbol{
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
			add(core.Symbol{
				Name:     symbol.Name,
				Address:  pltAddress,
				Size:     16,
				Kind:     core.SymbolFunction,
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
	if section := file.Section(".plt.sec"); section != nil && section.Addr != 0 {
		return section.Addr + uint64(index)*16, true
	}
	if section := file.Section(".plt"); section != nil && section.Addr != 0 {
		return section.Addr + uint64(index+1)*16, true
	}
	return 0, false
}

func elfSymbolKind(symbol elf.Symbol) core.SymbolKind {
	switch elf.ST_TYPE(symbol.Info) {
	case elf.STT_FUNC:
		return core.SymbolFunction
	case elf.STT_OBJECT:
		return core.SymbolObject
	default:
		return core.SymbolUnknown
	}
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
