package macho

import (
	"context"
	"debug/macho"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Loader loads Mach-O images. Thin images use their own header; universal
// (fat) images expose several architecture slices, and the loader selects one.
type Loader struct{}

var (
	_ core.Loader = Loader{}
)

const (
	machoCommandMain           = 0x80000028
	machoCommandFunctionStarts = 0x00000026
	machoSegmentText           = "__TEXT"
	machoAttrPureInstructions  = 0x80000000
	machoAttrSomeInstructions  = 0x00000400
	machoAttrDebug             = 0x02000000
	machoSymbolDebugMask       = 0xe0 // N_STAB
	machoSymbolTypeMask        = 0x0e // N_TYPE
	machoSymbolSection         = 0x0e // N_SECT
	machoSymbolExternal        = 0x01 // N_EXT
)

func (Loader) Kind() core.Format   { return core.FormatMachO }
func (Loader) Name() string        { return "Mach-O" }
func (Loader) Description() string { return core.FormatDescription(core.FormatMachO) }

// Match accepts a thin Mach-O magic, or a fat header that is structurally
// plausible. The fat magic is shared with Java class files, so the header must
// also pass fatHeaderPlausible instead of being claimed on the magic alone.
func (Loader) Match(prefix []byte) bool {
	if core.CheckFormat(prefix) == core.FormatMachO {
		return true
	}
	return fatHeaderPlausible(prefix)
}

// fatHeaderPlausible validates the fields a Java class file would not satisfy:
// a small architecture count and a known first CPU type.
func fatHeaderPlausible(prefix []byte) bool {
	if len(prefix) < 8+20 {
		return false
	}
	magic := binary.BigEndian.Uint32(prefix[:4])
	if magic != 0xcafebabe && magic != 0xcafebabf {
		return false
	}
	count := binary.BigEndian.Uint32(prefix[4:8])
	if count == 0 || count > 32 {
		return false
	}
	firstCPU := macho.Cpu(binary.BigEndian.Uint32(prefix[8:12]))
	switch firstCPU {
	case macho.Cpu386, macho.CpuAmd64, macho.CpuArm, macho.CpuArm64,
		macho.CpuPpc, macho.CpuPpc64:
		return true
	default:
		return false
	}
}

func (Loader) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, _, diagnostics, err := inspectMachO(r)
	if err != nil {
		return nil, err
	}
	return core.NewCheckResult(r, core.FormatMachO, target, image, diagnostics)
}

func (Loader) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, hints, diagnostics, err := inspectMachO(r)
	if err != nil {
		return nil, err
	}
	artifact, err := core.NewArtifact(ctx, r, core.FormatMachO, target, image, diagnostics)
	if err != nil {
		return nil, err
	}
	artifact.Hints = hints
	return artifact, nil
}

func inspectMachO(reader *core.ByteReader) (*core.Image, core.Target, *core.BoundaryHints, []core.Diagnostic, error) {
	file, sliceOffset, diagnostics, err := openMachO(reader)
	if err != nil {
		return nil, core.Target{}, nil, nil, err
	}
	defer file.Close()
	bits := 32
	if file.Magic == macho.Magic64 {
		bits = 64
	}
	target := core.Target{
		OS:           core.OSDarwin,
		Architecture: machoArchitecture(file.Cpu),
		Bits:         bits,
		ByteOrder:    core.ByteOrderOf(file.ByteOrder),
		ABI:          "macho",
		Machine:      fmt.Sprintf("%s/subcpu-0x%x", file.Cpu, file.SubCpu),
	}
	image := &core.Image{ObjectType: file.Type.String(), Sections: make([]core.Section, 0, len(file.Sections))}
	hints := &core.BoundaryHints{}
	textAddress, textOffset := uint64(0), uint64(0)
	var entry *uint64
	for _, load := range file.Loads {
		if segment, ok := load.(*macho.Segment); ok && segment.Name == machoSegmentText {
			textAddress, textOffset = segment.Addr, segment.Offset
		}
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}
		command := file.ByteOrder.Uint32(raw[:4])
		switch command {
		case machoCommandMain:
			if len(raw) >= 16 {
				offset := file.ByteOrder.Uint64(raw[8:16])
				if textAddress != 0 && offset >= textOffset {
					value := textAddress + offset - textOffset
					entry = &value
				}
			}
		case machoCommandFunctionStarts:
			if len(raw) >= 16 {
				dataOffset := file.ByteOrder.Uint32(raw[8:12])
				dataSize := file.ByteOrder.Uint32(raw[12:16])
				if data, err := reader.Bytes(sliceOffset+uint64(dataOffset), uint64(dataSize)); err == nil {
					hints.FunctionStarts = parseMachOFunctionStarts(data, textAddress)
				}
			}
		}
	}
	image.Entry = entry
	for _, section := range file.Sections {
		fileSize := section.Size
		// Mach-O SECTION_TYPE: S_ZEROFILL, S_GB_ZEROFILL and
		// S_THREAD_LOCAL_ZEROFILL have no file payload.
		switch section.Flags & 0xff {
		case 0x01, 0x0c, 0x12:
			fileSize = 0
		}
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Seg + "," + section.Name,
			FileOffset:     sliceOffset + uint64(section.Offset),
			FileSize:       fileSize,
			VirtualAddress: section.Addr,
			MemorySize:     section.Size,
			Flags:          uint64(section.Flags),
			Writable:       strings.HasPrefix(section.Seg, "__DATA"),
		})
		if fileSize == 0 {
			continue
		}
		if section.Flags&(machoAttrPureInstructions|machoAttrSomeInstructions) != 0 {
			hints.ExecSections = append(hints.ExecSections, core.Region{
				Address: section.Addr,
				Offset:  sliceOffset + uint64(section.Offset),
				Size:    fileSize,
			})
		}
	}
	if file.Symtab != nil {
		for _, item := range file.Symtab.Syms {
			if item.Type&machoSymbolDebugMask != 0 || item.Type&machoSymbolTypeMask != machoSymbolSection {
				continue
			}
			// N_SECT uses a one-based section ordinal. A function can start at
			// address zero in a relocatable object, and a data symbol can name
			// a zero-filled section with no bytes in the file.
			if item.Sect == 0 || int(item.Sect) > len(image.Sections) {
				continue
			}
			section := &image.Sections[int(item.Sect)-1]
			if section.Flags&machoAttrDebug != 0 || item.Value < section.VirtualAddress || item.Value-section.VirtualAddress >= section.MemorySize {
				continue
			}
			name := strings.TrimPrefix(item.Name, "_")
			if name == "" || isAssemblerTempSymbol(item) {
				continue
			}
			kind := sym.SymbolObject
			if section.FileSize > 0 && section.Flags&(machoAttrPureInstructions|machoAttrSomeInstructions) != 0 {
				kind = sym.SymbolFunction
			}
			image.Symbols = append(image.Symbols, sym.Symbol{
				Name:    name,
				Address: item.Value,
				Kind:    kind,
			})
		}
	}
	if imported, err := file.ImportedSymbols(); err == nil {
		for _, name := range imported {
			image.Symbols = append(image.Symbols, sym.Symbol{
				Name:     strings.TrimPrefix(name, "_"),
				Kind:     sym.SymbolUnknown,
				Imported: true,
			})
		}
	}
	if entry == nil {
		diagnostics = append(diagnostics, core.Diagnostic{
			Code:    "entry-point-unavailable",
			Message: "Mach-O LC_MAIN did not provide a usable entry point",
		})
	}
	return image, target, hints, diagnostics, nil
}

// LLVM emits local Mach-O labels named ltmpN, including section anchors that
// can share a function's address. Use the raw name to preserve source
// symbols such as _ltmp29.
func isAssemblerTempSymbol(item macho.Symbol) bool {
	suffix, ok := strings.CutPrefix(item.Name, "ltmp")
	return item.Type&machoSymbolExternal == 0 && ok && suffix != "" &&
		strings.Trim(suffix, "0123456789") == ""
}

// openMachO opens a thin or a universal image. A universal image exposes one
// slice per architecture; the loader picks ARM64 first, then AMD64, then the
// first slice, and returns the slice's file offset for all later reads.
func openMachO(reader *core.ByteReader) (*macho.File, uint64, []core.Diagnostic, error) {
	if core.CheckFormat(prefixOf(reader)) != core.FormatMachO {
		fat, err := macho.NewFatFile(reader)
		if err != nil {
			return nil, 0, nil, err
		}
		arch, err := selectFatArch(fat)
		if err != nil {
			_ = fat.Close()
			return nil, 0, nil, err
		}
		chosen := arch.File
		offset, cpu := uint64(arch.Offset), arch.Cpu
		count := len(fat.Arches)
		// FatFile.Close closes every slice. Detach the chosen one so the
		// caller's defer still closes it exactly once.
		arch.File = nil
		_ = fat.Close()
		return chosen, offset, []core.Diagnostic{{
			Code:    "fat-slice-selected",
			Message: fmt.Sprintf("selected the %s slice at file offset 0x%x from %d slices", cpu, offset, count),
		}}, nil
	}
	file, err := macho.NewFile(reader)
	if err != nil {
		return nil, 0, nil, err
	}
	return file, 0, nil, nil
}

func prefixOf(reader *core.ByteReader) []byte {
	size := uint64(core.ProbeSize)
	if uint64(reader.Size()) < size {
		size = uint64(reader.Size())
	}
	prefix, err := reader.Bytes(0, size)
	if err != nil {
		return nil
	}
	return prefix
}

// selectFatArch prefers ARM64 because Apple silicon is the current target,
// then AMD64, and otherwise the first slice in header order.
func selectFatArch(fat *macho.FatFile) (*macho.FatArch, error) {
	var arm64, amd64 *macho.FatArch
	for index := range fat.Arches {
		arch := &fat.Arches[index]
		switch arch.Cpu {
		case macho.CpuArm64:
			if arm64 == nil {
				arm64 = arch
			}
		case macho.CpuAmd64:
			if amd64 == nil {
				amd64 = arch
			}
		}
	}
	switch {
	case arm64 != nil:
		return arm64, nil
	case amd64 != nil:
		return amd64, nil
	case len(fat.Arches) > 0:
		return &fat.Arches[0], nil
	default:
		return nil, fmt.Errorf("universal Mach-O image has no slices")
	}
}

func parseMachOFunctionStarts(data []byte, base uint64) []uint64 {
	var result []uint64
	address := base
	for index := 0; index < len(data) && data[index] != 0; {
		delta, next := readULEB128(data[index:])
		if next <= 0 {
			break
		}
		index += next
		address += delta
		if address != 0 {
			result = append(result, address)
		}
	}
	return result
}

func readULEB128(data []byte) (uint64, int) {
	var value uint64
	var shift uint
	for index, current := range data {
		value |= uint64(current&0x7f) << shift
		if current&0x80 == 0 {
			return value, index + 1
		}
		shift += 7
		if shift >= 64 {
			return 0, 0
		}
	}
	return 0, 0
}

func machoArchitecture(cpu macho.Cpu) core.Architecture {
	switch cpu {
	case macho.CpuAmd64:
		return core.ArchAMD64
	case macho.Cpu386:
		return core.Arch386
	case macho.CpuArm64:
		return core.ArchARM64
	case macho.CpuArm:
		return core.ArchARM
	case macho.CpuPpc:
		return core.ArchPPC
	case macho.CpuPpc64:
		return core.ArchPPC64
	default:
		return core.ArchUnknown
	}
}
