// Package coff loads Windows COFF object files. A COFF object carries one code
// and one data section per translation unit, with the symbol table naming the
// functions the linker will place.
package coff

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Header sizes.
const (
	fileHeaderSize    = 20
	sectionHeaderSize = 40
	symbolSize        = 18
)

// Machine identifiers from the COFF specification.
const (
	machineI386  = 0x014c
	machineAMD64 = 0x8664
	machineARM   = 0x01c0
	machineARMNT = 0x01c4
	machineARM64 = 0xaa64
)

// section characteristics.
const (
	sectionCode        = 0x00000020
	sectionExecute     = 0x20000000
	sectionWrite       = 0x80000000
	symbolTypeMask     = 0x00000030
	symbolTypeFunction = 0x00000020
	storageClassStatic = 3
)

// Loader loads COFF object files.
type Loader struct{}

var _ core.Loader = Loader{}

func (Loader) Kind() core.Format   { return core.FormatCOFF }
func (Loader) Name() string        { return "COFF" }
func (Loader) Description() string { return "Windows COFF object files" }

// Match accepts a COFF object header. A COFF object has no magic: the machine
// identifier and the section count decide, and the PE loader claims an MZ
// image before this loader sees it.
func (Loader) Match(prefix []byte) bool {
	if len(prefix) < fileHeaderSize {
		return false
	}
	machine := binary.LittleEndian.Uint16(prefix)
	if !knownMachine(machine) {
		return false
	}
	sections := binary.LittleEndian.Uint16(prefix[2:])
	if sections == 0 || sections > 0x2000 {
		return false
	}
	optional := binary.LittleEndian.Uint16(prefix[16:])
	// A COFF object has no optional header. Windows keeps a small "bigobj"
	// header elsewhere; a value above 0x400 is not an object file.
	return optional <= 0x400
}

// knownMachine reports whether the identifier matches a supported architecture.
func knownMachine(machine uint16) bool {
	switch machine {
	case machineI386, machineAMD64, machineARM, machineARMNT, machineARM64:
		return true
	}
	return false
}

func (Loader) Check(reader *core.ByteReader) (*core.CheckResult, error) {
	artifact, err := Loader{}.Load(context.Background(), reader)
	if err != nil {
		return nil, err
	}
	return &core.CheckResult{
		Kind:       core.KindNative,
		Format:     core.FormatCOFF,
		Target:     artifact.Target,
		ObjectType: artifact.Image.ObjectType,
		Sections:   len(artifact.Image.Sections),
	}, nil
}

func (Loader) Load(ctx context.Context, reader *core.ByteReader) (*core.Artifact, error) {
	if reader == nil {
		return nil, fmt.Errorf("coff: nil reader")
	}
	header, err := reader.Bytes(0, fileHeaderSize)
	if err != nil || len(header) < fileHeaderSize {
		return nil, fmt.Errorf("coff: short file header")
	}
	machine := binary.LittleEndian.Uint16(header)
	sections := int(binary.LittleEndian.Uint16(header[2:]))
	symbolOffset := uint64(binary.LittleEndian.Uint32(header[8:]))
	symbolCount := int(binary.LittleEndian.Uint32(header[12:]))
	optionalSize := uint64(binary.LittleEndian.Uint16(header[16:]))

	target := core.Target{
		OS:           core.OSWindows,
		Architecture: architecture(machine),
		Bits:         bitsOf(machine),
		ByteOrder:    core.LittleEndian,
		ABI:          "coff",
		Machine:      machineName(machine),
	}
	if target.Architecture == core.ArchUnknown {
		return nil, fmt.Errorf("coff: unsupported machine 0x%04x", machine)
	}

	image := &core.Image{ObjectType: "COFF"}
	hints := &core.BoundaryHints{}
	tableOffset := fileHeaderSize + optionalSize
	// The string table sits after the symbol table and holds the section names
	// longer than eight bytes.
	names := readStringTable(reader, symbolOffset, symbolCount)
	// A relocatable object gives every section address zero, because the linker
	// assigns the addresses. The loader lays the sections out in file order so
	// their ranges stay distinct, and records the base of each section so a
	// symbol value, which is section relative, can be resolved.
	bases := make([]uint64, sections)
	namesOf := make([]string, sections)
	base := uint64(0)
	for index := 0; index < sections; index++ {
		entry, err := reader.Bytes(tableOffset+uint64(index)*sectionHeaderSize, sectionHeaderSize)
		if err != nil || len(entry) < sectionHeaderSize {
			return nil, fmt.Errorf("coff: short section header %d", index)
		}
		name := sectionName(entry, names)
		virtualSize := uint64(binary.LittleEndian.Uint32(entry[8:]))
		address := uint64(binary.LittleEndian.Uint32(entry[12:]))
		size := uint64(binary.LittleEndian.Uint32(entry[16:]))
		fileOffset := uint64(binary.LittleEndian.Uint32(entry[20:]))
		characteristics := binary.LittleEndian.Uint32(entry[36:])
		if virtualSize != 0 {
			size = virtualSize
		}
		namesOf[index] = name
		if size == 0 {
			// The section is BSS or a debug marker with no payload.
			continue
		}
		if address == 0 {
			address = base
		}
		base = address + ((size + 0xf) &^ 0xf)
		bases[index] = address
		image.Sections = append(image.Sections, core.Section{
			Name:           name,
			FileOffset:     fileOffset,
			FileSize:       uint64(binary.LittleEndian.Uint32(entry[16:])),
			VirtualAddress: address,
			MemorySize:     size,
			Flags:          uint64(characteristics),
			Writable:       characteristics&sectionWrite != 0,
		})
		if characteristics&(sectionCode|sectionExecute) != 0 {
			hints.ExecSections = append(hints.ExecSections, core.Region{
				Address: address,
				Offset:  fileOffset,
				Size:    uint64(binary.LittleEndian.Uint32(entry[16:])),
			})
		}
	}
	if symbolOffset != 0 && symbolCount > 0 {
		symbols, starts := readSymbols(reader, symbolOffset, symbolCount, names, bases, namesOf, image.Sections)
		image.Symbols = append(image.Symbols, symbols...)
		hints.FunctionStarts = append(hints.FunctionStarts, starts...)
	}
	if len(image.Sections) == 0 {
		return nil, fmt.Errorf("coff: object has no section with a payload")
	}
	return &core.Artifact{
		Kind:   core.KindNative,
		Format: core.FormatCOFF,
		Image:  image,
		Target: target,
		Hints:  hints,
	}, nil
}

// readStringTable reads the string table that follows the symbol table. Its
// first four bytes hold its own size, so a zero or absent table is empty.
func readStringTable(reader *core.ByteReader, symbolOffset uint64, symbolCount int) []byte {
	if symbolOffset == 0 {
		return nil
	}
	start := symbolOffset + uint64(symbolCount)*symbolSize
	header, err := reader.Bytes(start, 4)
	if err != nil || len(header) < 4 {
		return nil
	}
	size := binary.LittleEndian.Uint32(header)
	if size < 4 {
		return nil
	}
	data, err := reader.Bytes(start, uint64(size))
	if err != nil && len(data) < int(size) {
		return nil
	}
	// Index zero is the size field itself, so a name offset starts at four.
	return data
}

// sectionName returns the resolved name of one section header.
func sectionName(entry []byte, names []byte) string {
	raw := strings.TrimRight(string(entry[0:8]), "\x00")
	if strings.HasPrefix(raw, "/") && len(raw) > 1 {
		var index int
		if _, err := fmt.Sscanf(raw[1:], "%d", &index); err == nil && index >= 0 && index+4 <= len(names) {
			rest := names[index:]
			end := 0
			for end < len(rest) && rest[end] != 0 {
				end++
			}
			return string(rest[:end])
		}
	}
	return raw
}

// readSymbols reads the symbol table and returns the defined symbols together
// with the start addresses of the functions. A symbol value is relative to its
// section, so the section base is added.
func readSymbols(reader *core.ByteReader, offset uint64, count int, names []byte, bases []uint64, sectionNames []string, sections []core.Section) ([]sym.Symbol, []uint64) {
	result := make([]sym.Symbol, 0, count)
	starts := make([]uint64, 0)
	for index := 0; index < count; index++ {
		entry, err := reader.Bytes(offset+uint64(index)*symbolSize, symbolSize)
		if err != nil || len(entry) < symbolSize {
			break
		}
		name := symbolName(entry, names)
		value := uint64(binary.LittleEndian.Uint32(entry[8:]))
		sectionNumber := int16(binary.LittleEndian.Uint16(entry[12:]))
		storageClass := entry[16]
		sectionIndex := int(sectionNumber) - 1
		if sectionNumber <= 0 || storageClass == 0 {
			// An undefined or section-less symbol is an import reference.
			continue
		}
		address := value
		if sectionIndex < len(bases) {
			address += bases[sectionIndex]
		}
		kind := sym.SymbolObject
		if isSectionSymbol(entry, name, sectionIndex, sectionNames) {
			// The section symbol names the section and carries its start, not a
			// definition of its own.
			continue
		}
		if isFunctionSymbol(entry, sectionIndex, sections) {
			kind = sym.SymbolFunction
			starts = append(starts, address)
		}
		result = append(result, sym.Symbol{
			Name:    name,
			Address: address,
			Kind:    kind,
		})
	}
	return result, starts
}

// isSectionSymbol reports whether one entry is the static symbol that names its
// own section. The COFF writer emits one per section, with the section name as
// its name and the section start as its value.
func isSectionSymbol(entry []byte, name string, sectionIndex int, sectionNames []string) bool {
	if sectionIndex < 0 || sectionIndex >= len(sectionNames) {
		return false
	}
	if entry[16] != storageClassStatic {
		return false
	}
	return name == sectionNames[sectionIndex]
}

// isFunctionSymbol reports whether one symbol table entry names a function. The
// type field is zero in most object files, so the section decides: a symbol in a
// code section is a function, and the section symbol itself is not.
func isFunctionSymbol(entry []byte, sectionIndex int, sections []core.Section) bool {
	if sectionIndex < 0 || sectionIndex >= len(sections) {
		return false
	}
	symbolType := binary.LittleEndian.Uint16(entry[14:])
	if symbolType&symbolTypeMask == symbolTypeFunction {
		return true
	}
	if entry[16] == storageClassStatic {
		return false
	}
	return sections[sectionIndex].Flags&(sectionCode|sectionExecute) != 0
}

// symbolName resolves one symbol name. A name of eight bytes or fewer is inline;
// a longer name stores a zero first word and the offset into the string table in
// the second.
func symbolName(entry []byte, names []byte) string {
	if binary.LittleEndian.Uint32(entry[0:4]) == 0 {
		index := int(binary.LittleEndian.Uint32(entry[4:8]))
		if index >= 4 && index < len(names) {
			rest := names[index:]
			end := 0
			for end < len(rest) && rest[end] != 0 {
				end++
			}
			return string(rest[:end])
		}
		return ""
	}
	return strings.TrimRight(string(entry[0:8]), "\x00")
}

// architecture maps one machine identifier to an architecture.
func architecture(machine uint16) core.Architecture {
	switch machine {
	case machineI386:
		return core.Arch386
	case machineAMD64:
		return core.ArchAMD64
	case machineARM, machineARMNT:
		return core.ArchARM
	case machineARM64:
		return core.ArchARM64
	}
	return core.ArchUnknown
}

// bitsOf returns the address width of one machine.
func bitsOf(machine uint16) int {
	switch machine {
	case machineAMD64, machineARM64:
		return 64
	}
	return 32
}

// machineName renders one machine identifier.
func machineName(machine uint16) string {
	switch machine {
	case machineI386:
		return "i386"
	case machineAMD64:
		return "amd64"
	case machineARM:
		return "arm"
	case machineARMNT:
		return "armnt"
	case machineARM64:
		return "arm64"
	}
	return fmt.Sprintf("0x%04x", machine)
}
