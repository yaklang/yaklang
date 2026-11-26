package core

import (
	"sync"

	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Section preserves header facts. FileSize is zero for zero-filled sections.
// FileOffset is relative to Origin; VirtualAddress is an image address before
// runtime relocation. Flags retain their file-format-specific meaning, and
// Writable states the one fact consumers need in a format-neutral form.
type Section struct {
	Name           string `json:"name"`
	FileOffset     uint64 `json:"file_offset"`
	FileSize       uint64 `json:"file_size"`
	VirtualAddress uint64 `json:"virtual_address"`
	MemorySize     uint64 `json:"memory_size"`
	Flags          uint64 `json:"flags"`
	// Writable reports whether the section can change while the program runs.
	// A loader sets it from its own section attributes, so a consumer never has to
	// recognize section names.
	Writable bool `json:"writable,omitempty"`
}

// Relocation states the value a loader writes at an address before the program
// runs. A position-independent image leaves zero in the file and relies on the
// dynamic linker, so reading the bytes alone is not enough. A relocatable object
// records no value at all; Symbol names the callee the relocation refers to.
type Relocation struct {
	Address uint64 `json:"address"`
	Value   uint64 `json:"value"`
	Symbol  string `json:"symbol,omitempty"`
}

// Image is a concrete, format-neutral address-space view. Loaders fill it from
// their format headers; consumers do not need a format-specific image type.
// EntryPoint reports whether a header-derived entry address is available.
type Image struct {
	ObjectType  string          `json:"object_type"`
	Base        uint64          `json:"image_base,omitempty"`
	Entry       *uint64         `json:"entry_point,omitempty"`
	Sections    []Section       `json:"sections"`
	Symbols     []symbol.Symbol `json:"symbols,omitempty"`
	Relocations []Relocation    `json:"relocations,omitempty"`

	// byAddress indexes Symbols by address, built on first use.
	once      sync.Once
	byAddress map[uint64][]symbol.Symbol
}

// SetSymbols replaces the symbol table during artifact construction. Call it
// before publishing the image to concurrent readers; it invalidates the lazy
// address index when a metadata layer adds or renames symbols.
func (i *Image) SetSymbols(symbols []symbol.Symbol) {
	i.Symbols = symbols
	i.once = sync.Once{}
	i.byAddress = nil
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

// SectionAt returns the section containing address. Zero-sized sections never
// match.
func (i *Image) SectionAt(address uint64) (*Section, bool) {
	if i == nil {
		return nil, false
	}
	// A TLS .tbss can share virtual addresses with the sections that follow it;
	// its bytes are not in the file. Prefer a section that can supply bytes.
	var zeroFill *Section
	for idx := range i.Sections {
		section := &i.Sections[idx]
		if address < section.VirtualAddress || address-section.VirtualAddress >= section.MemorySize {
			continue
		}
		if address-section.VirtualAddress < section.FileSize {
			return section, true
		}
		if zeroFill == nil {
			zeroFill = section
		}
	}
	return zeroFill, zeroFill != nil
}

// RelocationAt returns the relocation that writes this address.
func (i *Image) RelocationAt(address uint64) (Relocation, bool) {
	if i == nil {
		return Relocation{}, false
	}
	for _, relocation := range i.Relocations {
		if relocation.Address == address {
			return relocation, true
		}
	}
	return Relocation{}, false
}

// InferredSymbolSize returns the byte range a symbol covers when its format
// records no size. Mach-O symbol entries carry only an address, so the next
// symbol in the same section bounds the range and the section end bounds the
// last symbol. A recorded size is returned unchanged.
func (i *Image) InferredSymbolSize(entry symbol.Symbol) uint64 {
	if i == nil || entry.Size > 0 {
		return entry.Size
	}
	section, ok := i.SectionAt(entry.Address)
	if !ok {
		return 0
	}
	limit := section.VirtualAddress + section.FileSize
	if limit <= entry.Address {
		limit = section.VirtualAddress + section.MemorySize
	}
	next := limit
	for _, other := range i.Symbols {
		if other.Imported || other.Address <= entry.Address {
			continue
		}
		if other.Address-section.VirtualAddress >= section.MemorySize {
			continue
		}
		if other.Address < next {
			next = other.Address
		}
	}
	if next <= entry.Address {
		return 0
	}
	return next - entry.Address
}

// SymbolsAt returns every symbol defined at one address. Several symbols share an
// address: a mapping symbol marks the start of a run and the function name follows.
func (i *Image) SymbolsAt(address uint64) []symbol.Symbol {
	if i == nil {
		return nil
	}
	i.once.Do(func() {
		i.byAddress = make(map[uint64][]symbol.Symbol, len(i.Symbols))
		for _, entry := range i.Symbols {
			i.byAddress[entry.Address] = append(i.byAddress[entry.Address], entry)
		}
	})
	return i.byAddress[address]
}

// SymbolByName returns the first symbol with this name. A language layer keeps
// the compiler-encoded name in Mangled, so a caller may pass either the display
// name or the encoded one.
func (i *Image) SymbolByName(name string) (symbol.Symbol, bool) {
	if i == nil || name == "" {
		return symbol.Symbol{}, false
	}
	for _, entry := range i.Symbols {
		if entry.Name == name {
			return entry, true
		}
	}
	for _, entry := range i.Symbols {
		if entry.Mangled == name {
			return entry, true
		}
	}
	return symbol.Symbol{}, false
}

// SymbolAt returns the symbol at address.
func (i *Image) SymbolAt(address uint64) (symbol.Symbol, bool) {
	if i == nil {
		return symbol.Symbol{}, false
	}
	return symbol.At(i.Symbols, address)
}
