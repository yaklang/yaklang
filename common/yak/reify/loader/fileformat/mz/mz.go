// Package mz loads MS-DOS MZ executables and COM images. Both target real mode:
// sixteen-bit addresses, no paging and no position-independent code.
package mz

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Image base for real-mode images. A COM file is loaded at 0x100 inside the
// program segment, and an MZ image at its own relocation base.
const comLoadAddress = 0x100

// Loader loads MS-DOS executables.
type Loader struct{}

var _ core.Loader = Loader{}

func (Loader) Kind() core.Format   { return core.FormatMZ }
func (Loader) Name() string        { return "MZ" }
func (Loader) Description() string { return "MS-DOS MZ executables and COM images" }

// Match accepts an MZ header whose stub does not name a PE signature. A COM
// image has no header at all, so it needs an explicit format selection.
func (Loader) Match(prefix []byte) bool {
	return core.CheckFormat(prefix) == core.FormatMZ && !peSignature(prefix)
}

// peSignature reports whether the DOS stub points at a PE signature.
func peSignature(prefix []byte) bool {
	if len(prefix) < 0x40 {
		return false
	}
	offset := binary.LittleEndian.Uint32(prefix[0x3c:])
	return offset >= 0x40 && offset < 0x1000
}

func (Loader) Check(reader *core.ByteReader) (*core.CheckResult, error) {
	artifact, err := Loader{}.Load(context.Background(), reader)
	if err != nil {
		return nil, err
	}
	return &core.CheckResult{
		Kind:       core.KindNative,
		Format:     core.FormatMZ,
		Target:     artifact.Target,
		ObjectType: artifact.Image.ObjectType,
		Sections:   len(artifact.Image.Sections),
	}, nil
}

func (Loader) Load(ctx context.Context, reader *core.ByteReader) (*core.Artifact, error) {
	if reader == nil {
		return nil, fmt.Errorf("mz: nil reader")
	}
	probe := uint64(core.ProbeSize)
	if limit := uint64(reader.Size()); probe > limit {
		probe = limit
	}
	head, err := reader.Bytes(0, probe)
	if err != nil && len(head) < 2 {
		return nil, fmt.Errorf("mz: cannot read header: %w", err)
	}
	target := core.Target{
		OS:           core.OSDOS,
		Architecture: core.Arch8086,
		Bits:         16,
		ByteOrder:    core.LittleEndian,
		ABI:          "mz",
		Machine:      "8086",
	}
	// A COM image has no header: the program starts at offset zero and the
	// whole file is the image.
	if !isMZ(head) {
		return loadCOM(reader, target)
	}
	return loadMZ(reader, target)
}

// isMZ reports whether the header carries the MZ signature.
func isMZ(head []byte) bool {
	return len(head) >= 2 && head[0] == 'M' && head[1] == 'Z'
}

// loadCOM builds an artifact for one COM image.
func loadCOM(reader *core.ByteReader, target core.Target) (*core.Artifact, error) {
	size := uint64(reader.Size())
	if size == 0 {
		return nil, fmt.Errorf("mz: empty COM image")
	}
	total := comLoadAddress + size
	image := &core.Image{
		ObjectType: "COM",
		Sections: []core.Section{{
			Name:           "code",
			FileOffset:     0,
			FileSize:       size,
			VirtualAddress: comLoadAddress,
			MemorySize:     size,
			Flags:          0x5, // read and execute
		}},
		Symbols: []sym.Symbol{{
			Name:    "start",
			Address: comLoadAddress,
			Size:    size,
			Kind:    sym.SymbolFunction,
		}},
	}
	// A COM image is a single code and data segment: the entry point is the
	// first byte and execution runs to the end of the file.
	entry := uint64(comLoadAddress)
	image.Entry = &entry
	hints := &core.BoundaryHints{
		ExecSections: []core.Region{{Address: comLoadAddress, Offset: 0, Size: size}},
	}
	artifact := &core.Artifact{
		Kind:   core.KindNative,
		Format: core.FormatMZ,
		Image:  image,
		Target: target,
		Hints:  hints,
	}
	_ = total
	return artifact, nil
}

// loadMZ builds an artifact for one MZ executable.
func loadMZ(reader *core.ByteReader, target core.Target) (*core.Artifact, error) {
	header, err := reader.Bytes(0, 28)
	if err != nil && len(header) < 28 {
		return nil, fmt.Errorf("mz: short header: %w", err)
	}
	if len(header) < 28 {
		return nil, fmt.Errorf("mz: short header: %d bytes", len(header))
	}
	order := binary.LittleEndian
	lastPage := int(order.Uint16(header[2:]))
	pages := int(order.Uint16(header[4:]))
	relocations := int(order.Uint16(header[6:]))
	headerSize := uint64(order.Uint16(header[8:])) * 16
	minExtra := uint64(order.Uint16(header[10:]))
	maxExtra := uint64(order.Uint16(header[12:]))
	initialSS := uint64(order.Uint16(header[14:]))
	initialSP := order.Uint16(header[16:])
	initialIP := uint64(order.Uint16(header[20:]))
	relocationOffset := uint64(order.Uint16(header[24:]))

	if pages == 0 || headerSize == 0 {
		return nil, fmt.Errorf("mz: header declares no pages or no header size")
	}
	// The last page holds lastPage bytes unless it is zero, which means a full
	// page.
	fileSize := uint64(pages-1)*512 + uint64(lastPage)
	if lastPage == 0 {
		fileSize = uint64(pages) * 512
	}
	if limit := uint64(reader.Size()); fileSize > limit {
		fileSize = limit
	}
	if headerSize >= fileSize {
		return nil, fmt.Errorf("mz: header size %d covers the whole file %d", headerSize, fileSize)
	}
	imageSize := fileSize - headerSize
	// The relocation base: the segment the loader assigns. Zero keeps the
	// offsets relative to the load module.
	image := &core.Image{
		ObjectType: "MZ",
		Sections: []core.Section{{
			Name:           "code",
			FileOffset:     headerSize,
			FileSize:       imageSize,
			VirtualAddress: 0,
			MemorySize:     imageSize + minExtra*16,
			Flags:          0x5,
		}},
	}
	// The entry point is CS:IP. The segment registers are relocated at load
	// time, so only the offset is a stable image address.
	entry := initialIP
	image.Entry = &entry
	if relocations > 0 && relocationOffset > 0 && relocationOffset < fileSize {
		image.Symbols = append(image.Symbols, relocationSymbols(reader, relocationOffset, relocations)...)
	}
	hints := &core.BoundaryHints{
		ExecSections: []core.Region{{Address: 0, Offset: headerSize, Size: imageSize}},
	}
	_ = initialSS
	_ = initialSP
	_ = maxExtra
	return &core.Artifact{
		Kind:   core.KindNative,
		Format: core.FormatMZ,
		Image:  image,
		Target: target,
		Hints:  hints,
	}, nil
}

// relocationSymbols reads the relocation table. Each entry names a segment and
// an offset inside the load module; the loader adds the base segment there, so
// the entry marks a far pointer the program uses.
func relocationSymbols(reader *core.ByteReader, offset uint64, count int) []sym.Symbol {
	data, err := reader.Bytes(offset, uint64(count*4))
	if err != nil && len(data) < count*4 {
		return nil
	}
	result := make([]sym.Symbol, 0, count)
	order := binary.LittleEndian
	for index := 0; index < count; index++ {
		entry := data[index*4:]
		if len(entry) < 4 {
			break
		}
		segment := order.Uint16(entry)
		location := order.Uint16(entry[2:])
		result = append(result, sym.Symbol{
			Name:    fmt.Sprintf("reloc_%d", index),
			Address: uint64(location),
			Kind:    sym.SymbolObject,
		})
		_ = segment
	}
	return result
}
