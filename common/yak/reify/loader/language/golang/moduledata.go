package golang

import (
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// A stripped Go image keeps no symbol that names runtime.text, and the Go 1.20
// function table stores entry offsets relative to it instead of the address
// itself. runtime.moduledata still holds the address, and the linker places it
// in a small data section. Reading that structure is what makes a stripped Go
// image usable: without the base every recovered name gets the wrong address.
//
// The field offsets below cover the layout every Go release since 1.18 uses for
// the fields this file reads. Validation checks every table pointer against the
// function table header, so a false candidate is rejected rather than trusted.
const (
	moduleDataHeaderOffset    = 0
	moduleDataFuncNameOffset  = 8
	moduleDataCUTabOffset     = 32
	moduleDataFileTabOffset   = 56
	moduleDataPCTabOffset     = 80
	moduleDataPCLNTableOffset = 104
	moduleDataFTabOffset      = 128
	moduleDataFTabLenOffset   = 136
	moduleDataMinPCOffset     = 160
	moduleDataMaxPCOffset     = 168
	moduleDataTextOffset      = 176
	moduleDataETextOffset     = 184
	moduleDataMinimumSize     = moduleDataETextOffset + 8
)

// moduleData is the part of runtime.moduledata this layer needs. Types and
// itabs live after fields whose position changed between releases, so they are
// not read here.
type moduleData struct {
	Address   uint64
	Text      uint64
	EText     uint64
	MinPC     uint64
	MaxPC     uint64
	FuncCount uint64
	Source    string
}

// pclnTables records where each function-table sub-table lives. runtime.moduledata
// repeats these addresses, which is what validation compares.
type pclnTables struct {
	headerAddr uint64
	funcNames  uint64
	cuTab      uint64
	fileTab    uint64
	pcTab      uint64
	pclnTable  uint64
	funcCount  uint64
}

func (t pclnTables) valid() bool {
	return t.headerAddr != 0 && t.funcCount > 0
}

// readPclnTables reads the header words of the Go 1.18+ function table. word
// returns the value at one pointer-sized header slot.
func readPclnTables(headerAddr uint64, funcCount uint64, word func(int) uint64) pclnTables {
	return pclnTables{
		headerAddr: headerAddr,
		funcNames:  word(3),
		cuTab:      word(4),
		fileTab:    word(5),
		pcTab:      word(6),
		pclnTable:  word(7),
		funcCount:  funcCount,
	}
}

// recoverModuleData searches for runtime.moduledata and returns its text range.
// A named section is the cheapest source; a stripped image from another format
// falls back to scanning the data sections for the header pointer.
func recoverModuleData(a *core.Artifact, tables pclnTables, order binary.ByteOrder) (moduleData, bool) {
	if !tables.valid() {
		return moduleData{}, false
	}
	if entry, ok := a.Image.SymbolByName("runtime.firstmoduledata"); ok && entry.Address != 0 {
		if data, err := readImageRange(a, entry.Address, moduleDataMinimumSize); err == nil {
			if found, ok := parseModuleData(data, tables, order, entry.Address, "symbol runtime.firstmoduledata"); ok {
				return found, true
			}
		}
	}
	// .go.module holds the linker-built module data in ELF images. Try it first,
	// then scan the remaining data sections, because an older linker or another
	// format may put the structure in .noptrdata or .data.rel.ro.
	for _, section := range a.Image.Sections {
		if section.Name != ".go.module" || section.FileSize < moduleDataMinimumSize {
			continue
		}
		if found, ok := scanModuleDataSection(a, section, tables, order); ok {
			return found, true
		}
	}
	for _, section := range a.Image.Sections {
		if section.Name == ".go.module" || section.FileSize < moduleDataMinimumSize || section.FileSize > maxMetadataSize {
			continue
		}
		if !moduleDataSourceSection(section) {
			continue
		}
		if found, ok := scanModuleDataSection(a, section, tables, order); ok {
			return found, true
		}
	}
	return moduleData{}, false
}

// moduleDataSourceSection reports whether a section can hold module data. The
// structure is static data, so code and compiler tables are excluded.
func moduleDataSourceSection(section core.Section) bool {
	switch section.Name {
	case ".noptrdata", ".data", ".data.rel.ro", ".rdata", ".go.module", "__data", "__const":
		return true
	}
	return false
}

func scanModuleDataSection(a *core.Artifact, section core.Section, tables pclnTables, order binary.ByteOrder) (moduleData, bool) {
	data, err := a.Reader.Bytes(section.FileOffset, section.FileSize)
	if err != nil {
		return moduleData{}, false
	}
	for offset := 0; offset+moduleDataMinimumSize <= len(data); offset += 8 {
		address := section.VirtualAddress + uint64(offset)
		if order.Uint64(data[offset:]) != tables.headerAddr {
			continue
		}
		if found, ok := parseModuleData(data[offset:], tables, order, address, "scan "+section.Name); ok {
			return found, true
		}
	}
	return moduleData{}, false
}

// parseModuleData validates one candidate structure. Every table pointer and
// the function count must match the header, which is what separates module data
// from a word that happens to equal the header address.
func parseModuleData(data []byte, tables pclnTables, order binary.ByteOrder, address uint64, source string) (moduleData, bool) {
	if len(data) < moduleDataMinimumSize {
		return moduleData{}, false
	}
	word := func(offset int) uint64 { return order.Uint64(data[offset:]) }
	if word(moduleDataHeaderOffset) != tables.headerAddr {
		return moduleData{}, false
	}
	if word(moduleDataFuncNameOffset) != tables.headerAddr+tables.funcNames ||
		word(moduleDataCUTabOffset) != tables.headerAddr+tables.cuTab ||
		word(moduleDataFileTabOffset) != tables.headerAddr+tables.fileTab ||
		word(moduleDataPCTabOffset) != tables.headerAddr+tables.pcTab ||
		word(moduleDataPCLNTableOffset) != tables.headerAddr+tables.pclnTable {
		return moduleData{}, false
	}
	if word(moduleDataFTabOffset) != tables.headerAddr+tables.pclnTable {
		return moduleData{}, false
	}
	if word(moduleDataFTabLenOffset) != tables.funcCount+1 {
		return moduleData{}, false
	}
	text := word(moduleDataTextOffset)
	etext := word(moduleDataETextOffset)
	minPC := word(moduleDataMinPCOffset)
	maxPC := word(moduleDataMaxPCOffset)
	if text == 0 || etext <= text || minPC < text || maxPC >= etext {
		return moduleData{}, false
	}
	return moduleData{
		Address:   address,
		Text:      text,
		EText:     etext,
		MinPC:     minPC,
		MaxPC:     maxPC,
		FuncCount: tables.funcCount,
		Source:    source,
	}, true
}

func readImageRange(a *core.Artifact, address, size uint64) ([]byte, error) {
	region, err := a.RegionAt(address, size)
	if err != nil {
		return nil, err
	}
	reader, err := a.RegionReader(region)
	if err != nil {
		return nil, err
	}
	return reader.Bytes(0, size)
}

// textStart resolves the address the function table offsets are relative to.
// The order is: the runtime.text symbol, runtime.moduledata, the .text section,
// then the first executable region. The last two are layout guesses and only
// apply when no compiler-owned structure records the truth.
func textStart(a *core.Artifact, tables pclnTables, order binary.ByteOrder) (uint64, error) {
	if entry, ok := a.Image.SymbolByName("runtime.text"); ok && entry.Address != 0 {
		return entry.Address, nil
	}
	if found, ok := recoverModuleData(a, tables, order); ok {
		return found.Text, nil
	}
	for _, section := range a.Image.Sections {
		if section.Name == ".text" && section.VirtualAddress != 0 {
			return section.VirtualAddress, nil
		}
	}
	if a.Hints != nil && len(a.Hints.ExecSections) > 0 {
		return a.Hints.ExecSections[0].Address, nil
	}
	return 0, fmt.Errorf("Go text base is not recoverable: no runtime.text symbol, no module data and no executable section")
}
