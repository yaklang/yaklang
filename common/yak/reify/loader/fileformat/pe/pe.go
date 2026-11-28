package pe

import (
	"context"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// PE loads Portable Executable images. The subsystem is not always Windows:
// firmware and other execution environments also use the PE container.
type Loader struct{}

var (
	_ core.Loader = Loader{}
)

const (
	peSectionCode         = 0x00000020
	peSectionExecute      = 0x20000000
	peSymbolTypeFunction  = 0x20
	peDirectoryException  = 3
	peExceptionEntrySize  = 12 // RUNTIME_FUNCTION for x86-64
	peArm64ExceptionEntry = 8  // RUNTIME_FUNCTION for ARM64
)

func (Loader) Kind() core.Format   { return core.FormatPE }
func (Loader) Name() string        { return "PE" }
func (Loader) Description() string { return core.FormatDescription(core.FormatPE) }
func (Loader) Match(prefix []byte) bool {
	// A PE image starts with an MZ stub and carries the PE signature at the
	// offset the stub records. An MZ executable has the same first two bytes, so
	// the stub header decides: the signature offset must point inside the file
	// and past the DOS header.
	return core.CheckFormat(prefix) == core.FormatMZ && peSignaturePlausible(prefix)
}

// peSignaturePlausible reports whether the DOS stub names a signature offset
// that a PE image can occupy.
func peSignaturePlausible(prefix []byte) bool {
	if len(prefix) < 0x40 {
		return false
	}
	offset := int(binary.LittleEndian.Uint32(prefix[0x3c:]))
	return offset >= 0x40 && offset < 0x1000
}

func (Loader) Check(r *core.ByteReader) (*core.CheckResult, error) {
	image, target, _, diagnostics, err := inspectPE(r)
	if err != nil {
		return nil, err
	}
	return core.NewCheckResult(r, core.FormatPE, target, image, diagnostics)
}

func (Loader) Load(ctx context.Context, r *core.ByteReader) (*core.Artifact, error) {
	image, target, hints, diagnostics, err := inspectPE(r)
	if err != nil {
		return nil, err
	}
	artifact, err := core.NewArtifact(ctx, r, core.FormatPE, target, image, diagnostics)
	if err != nil {
		return nil, err
	}
	artifact.Hints = hints
	return artifact, nil
}

func inspectPE(reader *core.ByteReader) (*core.Image, core.Target, *core.BoundaryHints, []core.Diagnostic, error) {
	file, err := pe.NewFile(reader)
	if err != nil {
		return nil, core.Target{}, nil, nil, err
	}
	defer file.Close()
	target := core.Target{
		Architecture: peArchitecture(file.Machine),
		ByteOrder:    core.LittleEndian,
		Machine:      fmt.Sprintf("0x%04x", file.Machine),
	}
	image := &core.Image{Sections: make([]core.Section, 0, len(file.Sections))}
	hints := &core.BoundaryHints{}
	var entryRVA uint32
	var subsystem uint16
	var directories []pe.DataDirectory
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		target.Bits, image.Base, entryRVA = 32, uint64(header.ImageBase), header.AddressOfEntryPoint
		image.ObjectType, subsystem = "PE32", header.Subsystem
		directories = header.DataDirectory[:]
	case *pe.OptionalHeader64:
		target.Bits, image.Base, entryRVA = 64, header.ImageBase, header.AddressOfEntryPoint
		image.ObjectType, subsystem = "PE32+", header.Subsystem
		directories = header.DataDirectory[:]
	default:
		return nil, core.Target{}, nil, nil, fmt.Errorf("PE image has no supported optional header")
	}
	target.OS = peOS(subsystem)
	target.ABI = fmt.Sprintf("pe-subsystem-%d", subsystem)
	if managed, ok := cliHeaderDirectory(directories); ok {
		// A managed image stores IL bytecode where a native image stores
		// machine code. Decoding it as native code produces valid-looking
		// garbage, so the loader refuses instead of guessing.
		return nil, core.Target{}, nil, nil, fmt.Errorf(
			"PE image is a .NET assembly: the CLI header at RVA 0x%x holds IL bytecode, not machine code", managed)
	}
	if entryRVA != 0 {
		entry, err := Address(image.Base, entryRVA)
		if err != nil {
			return nil, core.Target{}, nil, nil, err
		}
		image.Entry = &entry
	}
	for _, section := range file.Sections {
		address, err := Address(image.Base, section.VirtualAddress)
		if err != nil {
			return nil, core.Target{}, nil, nil, fmt.Errorf("section %q: %w", section.Name, err)
		}
		memorySize := uint64(section.VirtualSize)
		if memorySize < uint64(section.Size) {
			memorySize = uint64(section.Size)
		}
		image.Sections = append(image.Sections, core.Section{
			Name:           section.Name,
			FileOffset:     uint64(section.Offset),
			FileSize:       uint64(section.Size),
			VirtualAddress: address,
			MemorySize:     memorySize,
			Flags:          uint64(section.Characteristics),
			Writable:       section.Characteristics&pe.IMAGE_SCN_MEM_WRITE != 0,
		})
		if section.Size == 0 {
			continue
		}
		if section.Characteristics&peSectionCode != 0 || section.Characteristics&peSectionExecute != 0 {
			hints.ExecSections = append(hints.ExecSections, core.Region{
				Address: address,
				Offset:  uint64(section.Offset),
				Size:    uint64(section.Size),
			})
		}
	}
	parsePECOFFSymbols(file, image)
	parsePEExports(reader, file, image, directories, image.Base)
	parsePEExceptionTable(reader, file, &target, image, directories, hints)
	if len(image.Symbols) > 0 {
		seen := make(map[uint64]bool, len(image.Symbols))
		for _, item := range image.Symbols {
			if item.Kind != sym.SymbolFunction || item.Imported || item.Address == 0 || seen[item.Address] {
				continue
			}
			seen[item.Address] = true
			hints.FunctionStarts = append(hints.FunctionStarts, item.Address)
		}
	}
	hints.FunctionStarts = uniqueAddresses(hints.FunctionStarts)
	parsePEImports(reader, file, image, directories, target.Bits == 64)
	return image, target, hints, nil, nil
}

// cliHeaderDirectory reports the RVA of the CLI header directory. Directory
// entry fourteen is the COM descriptor, which only a managed image fills.
func cliHeaderDirectory(directories []pe.DataDirectory) (uint32, bool) {
	const cliDirectory = 14
	if len(directories) <= cliDirectory {
		return 0, false
	}
	directory := directories[cliDirectory]
	if directory.VirtualAddress == 0 || directory.Size == 0 {
		return 0, false
	}
	return directory.VirtualAddress, true
}

func Address(base uint64, rva uint32) (uint64, error) {
	if uint64(rva) > math.MaxUint64-base {
		return 0, fmt.Errorf("PE image base plus RVA overflows")
	}
	return base + uint64(rva), nil
}

func parsePECOFFSymbols(file *pe.File, image *core.Image) {
	seen := make(map[uint64]bool)
	for _, item := range file.Symbols {
		if item.SectionNumber <= 0 || int(item.SectionNumber) > len(file.Sections) {
			continue
		}
		section := file.Sections[item.SectionNumber-1]
		if section.Characteristics&(peSectionCode|peSectionExecute) == 0 {
			continue
		}
		// A COFF value is an offset inside its section, and a linked image adds
		// the image base to the section address. Requiring a non-zero value would
		// drop the first function of every section.
		address := image.Base + uint64(section.VirtualAddress) + uint64(item.Value)
		if address == 0 || seen[address] {
			continue
		}
		seen[address] = true
		image.Symbols = append(image.Symbols, sym.Symbol{
			Name:    item.Name,
			Address: address,
			Kind:    sym.SymbolFunction,
		})
	}
}

// parsePEExports reads the export directory so exported functions get names and
// exact addresses even when COFF symbols were stripped.
func parsePEExports(reader *core.ByteReader, file *pe.File, image *core.Image, directories []pe.DataDirectory, base uint64) {
	if len(directories) <= pe.IMAGE_DIRECTORY_ENTRY_EXPORT {
		return
	}
	directory := directories[pe.IMAGE_DIRECTORY_ENTRY_EXPORT]
	if directory.VirtualAddress == 0 || directory.Size < 40 {
		return
	}
	data, err := peReadRVA(reader, file, directory.VirtualAddress, 40)
	if err != nil || len(data) < 40 {
		return
	}
	order := binary.LittleEndian
	numberOfFunctions := uint64(order.Uint32(data[20:24]))
	numberOfNames := uint64(order.Uint32(data[24:28]))
	functionsRVA := order.Uint32(data[28:32])
	namesRVA := order.Uint32(data[32:36])
	ordinalsRVA := order.Uint32(data[36:40])
	const maxExportEntries = 1 << 20
	if numberOfFunctions == 0 || numberOfNames == 0 || numberOfNames > numberOfFunctions || numberOfFunctions > maxExportEntries {
		return
	}
	names, err := peReadRVA(reader, file, namesRVA, uint32(numberOfNames*4))
	if err != nil || uint64(len(names)) < numberOfNames*4 {
		return
	}
	functions, err := peReadRVA(reader, file, functionsRVA, uint32(numberOfFunctions*4))
	if err != nil || uint64(len(functions)) < numberOfFunctions*4 {
		return
	}
	ordinals, err := peReadRVA(reader, file, ordinalsRVA, uint32(numberOfNames*2))
	if err != nil || uint64(len(ordinals)) < numberOfNames*2 {
		return
	}
	var strBuf [512]byte
	for index := uint64(0); index < numberOfNames; index++ {
		nameRVA := order.Uint32(names[index*4:])
		ordinal := uint64(order.Uint16(ordinals[index*2:]))
		if ordinal >= numberOfFunctions {
			continue
		}
		functionRVA := order.Uint32(functions[ordinal*4:])
		if functionRVA == 0 {
			continue
		}
		name, err := peReadCString(reader, file, nameRVA, strBuf[:])
		if err != nil || name == "" {
			continue
		}
		address, err := Address(base, functionRVA)
		if err != nil {
			continue
		}
		image.Symbols = append(image.Symbols, sym.Symbol{Name: name, Address: address, Kind: sym.SymbolFunction})
	}
}

// parsePEExceptionTable reads the pdata RUNTIME_FUNCTION table. Each entry
// starts at a function's begin address, which gives exact boundaries without
// disassembly.
func parsePEExceptionTable(reader *core.ByteReader, file *pe.File, target *core.Target, image *core.Image, directories []pe.DataDirectory, hints *core.BoundaryHints) {
	if len(directories) <= peDirectoryException {
		return
	}
	entrySize := uint32(peExceptionEntrySize)
	if target.Architecture == core.ArchARM64 {
		entrySize = peArm64ExceptionEntry
	}
	directory := directories[peDirectoryException]
	if directory.VirtualAddress == 0 || directory.Size < entrySize {
		return
	}
	data, err := peReadRVA(reader, file, directory.VirtualAddress, directory.Size)
	if err != nil {
		return
	}
	existing := make(map[uint64]bool, len(image.Symbols))
	for _, item := range image.Symbols {
		if item.Imported || item.Address == 0 {
			continue
		}
		existing[item.Address] = true
	}
	seen := make(map[uint64]bool, len(data)/int(entrySize))
	for offset := 0; offset+int(entrySize) <= len(data); offset += int(entrySize) {
		begin := uint32(binary.LittleEndian.Uint32(data[offset:]))
		if begin == 0 {
			continue
		}
		address, err := Address(image.Base, begin)
		if err != nil || seen[address] {
			continue
		}
		seen[address] = true
		hints.FunctionStarts = append(hints.FunctionStarts, address)
		if !existing[address] {
			existing[address] = true
			image.Symbols = append(image.Symbols, sym.Symbol{
				Name:    fmt.Sprintf("sub_0x%x", address),
				Address: address,
				Kind:    sym.SymbolFunction,
			})
		}
	}
}

// parsePEImports reads the PE import directory directly from bounded RVA slices,
// avoiding stdlib pe.File.ImportedSymbols which reads the entire containing
// section (often >100 MB .rdata) into memory once per imported DLL.
func parsePEImports(reader *core.ByteReader, file *pe.File, image *core.Image, directories []pe.DataDirectory, isPE64 bool) {
	if len(directories) <= pe.IMAGE_DIRECTORY_ENTRY_IMPORT {
		return
	}
	directory := directories[pe.IMAGE_DIRECTORY_ENTRY_IMPORT]
	if directory.VirtualAddress == 0 || directory.Size < 20 {
		return
	}
	const (
		maxImportDirBytes = 4 << 20
		maxImportDLLs     = 1 << 14
		maxImportSymbols  = 1 << 20
		thunkChunkSize    = 4096
	)
	readLen := directory.Size
	if readLen > maxImportDirBytes {
		readLen = maxImportDirBytes
	}
	dirData, err := peReadRVA(reader, file, directory.VirtualAddress, readLen)
	if err != nil || len(dirData) < 20 {
		return
	}
	order := binary.LittleEndian
	step := uint32(4)
	if isPE64 {
		step = 8
	}
	var (
		thunkBuf [thunkChunkSize]byte
		strBuf   [512]byte
		total    int
	)
	descriptors := 0
	for off := 0; off+20 <= len(dirData) && descriptors < maxImportDLLs; off += 20 {
		descriptors++
		origFirstThunk := order.Uint32(dirData[off:])
		nameRVA := order.Uint32(dirData[off+12:])
		firstThunk := order.Uint32(dirData[off+16:])
		if origFirstThunk == 0 && nameRVA == 0 && firstThunk == 0 {
			break
		}
		if origFirstThunk == 0 {
			// Match stdlib debug/pe behavior when OriginalFirstThunk is absent,
			// while still falling back if Name is present? Only use FirstThunk when valid.
			origFirstThunk = firstThunk
		}
		if origFirstThunk == 0 || nameRVA == 0 {
			continue
		}
		dll, err := peReadCString(reader, file, nameRVA, strBuf[:])
		if err != nil || dll == "" {
			continue
		}
		curRVA := origFirstThunk
		for total < maxImportSymbols {
			n, err := peReadRVABuf(reader, file, curRVA, thunkBuf[:])
			if err != nil || n < int(step) {
				break
			}
			usable := (n / int(step)) * int(step)
			done := false
			for i := 0; i < usable && total < maxImportSymbols; i += int(step) {
				var entry uint64
				if isPE64 {
					entry = order.Uint64(thunkBuf[i:])
					if entry == 0 {
						done = true
						break
					}
					if entry&(1<<63) != 0 {
						continue
					}
				} else {
					v := order.Uint32(thunkBuf[i:])
					if v == 0 {
						done = true
						break
					}
					if v&(1<<31) != 0 {
						continue
					}
					entry = uint64(v)
				}
				if entry > math.MaxUint32-2 {
					continue
				}
				fn, err := peReadCString(reader, file, uint32(entry)+2, strBuf[:])
				if err != nil || fn == "" {
					continue
				}
				image.Symbols = append(image.Symbols, sym.Symbol{
					Name:     fn,
					Kind:     sym.SymbolFunction,
					Imported: true,
					Library:  dll,
				})
				total++
			}
			if done || uint64(curRVA)+uint64(usable) > math.MaxUint32 {
				break
			}
			curRVA += uint32(usable)
		}
	}
}

// uniqueAddresses removes duplicates while keeping the first occurrence.
func uniqueAddresses(addresses []uint64) []uint64 {
	if len(addresses) < 2 {
		return addresses
	}
	seen := make(map[uint64]bool, len(addresses))
	result := addresses[:0]
	for _, address := range addresses {
		if seen[address] {
			continue
		}
		seen[address] = true
		result = append(result, address)
	}
	return result
}

func peFindRVASection(file *pe.File, rva uint32) (*pe.Section, uint64, uint64, error) {
	for _, section := range file.Sections {
		if rva < section.VirtualAddress || rva-section.VirtualAddress >= section.VirtualSize {
			continue
		}
		delta := uint64(rva - section.VirtualAddress)
		if delta >= uint64(section.Size) {
			return nil, 0, 0, fmt.Errorf("PE RVA 0x%x has no file data", rva)
		}
		avail := uint64(section.Size) - delta
		return section, delta, avail, nil
	}
	return nil, 0, 0, fmt.Errorf("PE RVA 0x%x is not in a section", rva)
}

func peReadRVA(reader *core.ByteReader, file *pe.File, rva, size uint32) ([]byte, error) {
	section, delta, avail, err := peFindRVASection(file, rva)
	if err != nil {
		return nil, err
	}
	readSize := uint64(size)
	if readSize > avail {
		readSize = avail
	}
	if readSize == 0 {
		return nil, fmt.Errorf("PE RVA 0x%x has no file data", rva)
	}
	if reader != nil {
		return reader.Bytes(uint64(section.Offset)+delta, readSize)
	}
	data, err := section.Data()
	if err != nil {
		return nil, err
	}
	if delta >= uint64(len(data)) {
		return nil, fmt.Errorf("PE RVA 0x%x has no file data", rva)
	}
	if delta+readSize > uint64(len(data)) {
		readSize = uint64(len(data)) - delta
	}
	return data[delta : delta+readSize], nil
}

func peReadRVABuf(reader *core.ByteReader, file *pe.File, rva uint32, dst []byte) (int, error) {
	section, delta, avail, err := peFindRVASection(file, rva)
	if err != nil {
		return 0, err
	}
	readSize := uint64(len(dst))
	if readSize > avail {
		readSize = avail
	}
	if readSize == 0 {
		return 0, fmt.Errorf("PE RVA 0x%x has no file data", rva)
	}
	if reader != nil {
		n, err := reader.ReadAt(dst[:int(readSize)], int64(uint64(section.Offset)+delta))
		if n > 0 {
			return n, nil
		}
		return 0, err
	}
	data, err := section.Data()
	if err != nil {
		return 0, err
	}
	if delta >= uint64(len(data)) {
		return 0, fmt.Errorf("PE RVA 0x%x has no file data", rva)
	}
	n := copy(dst, data[delta:])
	return n, nil
}

func peReadCString(reader *core.ByteReader, file *pe.File, rva uint32, scratch []byte) (string, error) {
	if len(scratch) == 0 {
		scratch = make([]byte, 512)
	}
	n, err := peReadRVABuf(reader, file, rva, scratch)
	if err != nil || n == 0 {
		return "", err
	}
	data := scratch[:n]
	end := 0
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[:end]), nil
}

func peArchitecture(machine uint16) core.Architecture {
	switch machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return core.ArchAMD64
	case pe.IMAGE_FILE_MACHINE_I386:
		return core.Arch386
	case pe.IMAGE_FILE_MACHINE_ARM64:
		return core.ArchARM64
	case pe.IMAGE_FILE_MACHINE_ARM, pe.IMAGE_FILE_MACHINE_ARMNT:
		return core.ArchARM
	default:
		return core.ArchUnknown
	}
}

func peOS(subsystem uint16) core.OperatingSystem {
	switch subsystem {
	case pe.IMAGE_SUBSYSTEM_NATIVE, pe.IMAGE_SUBSYSTEM_WINDOWS_GUI,
		pe.IMAGE_SUBSYSTEM_WINDOWS_CUI, pe.IMAGE_SUBSYSTEM_NATIVE_WINDOWS,
		pe.IMAGE_SUBSYSTEM_WINDOWS_CE_GUI, pe.IMAGE_SUBSYSTEM_WINDOWS_BOOT_APPLICATION:
		return core.OSWindows
	default:
		// PE also describes firmware and other execution environments.
		return core.OSUnknown
	}
}
