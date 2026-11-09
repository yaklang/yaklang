package core

// Section preserves header facts. FileSize is zero for zero-filled sections.
// FileOffset is relative to Origin; VirtualAddress is an image address before
// runtime relocation. Flags retain their file-format-specific meaning.
// BoundarySource identifies how a function boundary was discovered.
type BoundarySource string

const (
	BoundarySymbol    BoundarySource = "symbol"
	BoundaryEHFrame   BoundarySource = "eh_frame"
	BoundaryInitArray BoundarySource = "init_array"
	BoundaryPLT       BoundarySource = "plt"
	BoundaryEntry     BoundarySource = "entry"
	BoundaryCall      BoundarySource = "call_target"
	BoundaryScan      BoundarySource = "scan"
)

// FunctionBoundary is one recovered function range. Size is resolved after all
// sources have been merged; a zero size means the range ends at the next
// discovered boundary or the executable section end.
type FunctionBoundary struct {
	Address uint64         `json:"address"`
	Size    uint64         `json:"size,omitempty"`
	Name    string         `json:"name,omitempty"`
	Source  BoundarySource `json:"source"`
}

// BoundaryStats reports discovery counts. Duplicates is the number of
// addresses that were reported by more than one source.
type BoundaryStats struct {
	Symbol     int `json:"symbol"`
	EHFrame    int `json:"eh_frame"`
	InitArray  int `json:"init_array"`
	PLT        int `json:"plt"`
	Entry      int `json:"entry"`
	Call       int `json:"call_target"`
	Scan       int `json:"scan"`
	Duplicates int `json:"duplicates"`
}

type SymbolKind string

const (
	SymbolFunction SymbolKind = "function"
	SymbolObject   SymbolKind = "object"
	SymbolUnknown  SymbolKind = "unknown"
)

// Symbol is one named address from a file-format symbol or relocation table.
// Imported symbols may have Address zero when the format only names them.
type Symbol struct {
	Name     string     `json:"name"`
	Address  uint64     `json:"address,omitempty"`
	Size     uint64     `json:"size,omitempty"`
	Kind     SymbolKind `json:"kind"`
	Imported bool       `json:"imported,omitempty"`
	Library  string     `json:"library,omitempty"`
}

type Section struct {
	Name           string `json:"name"`
	FileOffset     uint64 `json:"file_offset"`
	FileSize       uint64 `json:"file_size"`
	VirtualAddress uint64 `json:"virtual_address"`
	MemorySize     uint64 `json:"memory_size"`
	Flags          uint64 `json:"flags"`
}

// Image is a concrete, format-neutral address-space view. Loaders fill it from
// their format headers; consumers do not need a format-specific image type.
// EntryPoint reports whether a header-derived entry address is available.
type Image struct {
	ObjectType string    `json:"object_type"`
	Base       uint64    `json:"image_base,omitempty"`
	Entry      *uint64   `json:"entry_point,omitempty"`
	Sections   []Section `json:"sections"`
	Symbols    []Symbol  `json:"symbols,omitempty"`

	Functions     []FunctionBoundary `json:"functions,omitempty"`
	BoundaryStats BoundaryStats      `json:"boundary_stats,omitempty"`
}

func (i *Image) ImageBase() uint64 {
	if i == nil {
		return 0
	}
	return i.Base
}

func (i *Image) EntryPoint() (uint64, bool) {
	if i == nil || i.Entry == nil {
		return 0, false
	}
	return *i.Entry, true
}

// SymbolAt returns the symbol at address. An exact address match wins; a symbol
// with a size also matches addresses inside its range.
func (i *Image) SymbolAt(address uint64) (Symbol, bool) {
	if i == nil {
		return Symbol{}, false
	}
	for index := range i.Symbols {
		symbol := &i.Symbols[index]
		if symbol.Address == 0 {
			continue
		}
		if symbol.Address == address {
			return *symbol, true
		}
	}
	for index := range i.Symbols {
		symbol := &i.Symbols[index]
		if symbol.Imported || symbol.Size == 0 || symbol.Address == 0 || address < symbol.Address {
			continue
		}
		if address-symbol.Address < symbol.Size {
			return *symbol, true
		}
	}
	return Symbol{}, false
}

// SectionAt returns the section containing address. Zero-sized sections never
// match.
func (i *Image) SectionAt(address uint64) (*Section, bool) {
	if i == nil {
		return nil, false
	}
	for idx := range i.Sections {
		section := &i.Sections[idx]
		if address >= section.VirtualAddress && address-section.VirtualAddress < section.MemorySize {
			return section, true
		}
	}
	return nil, false
}
