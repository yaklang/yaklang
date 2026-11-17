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
// dynamic linker, so reading the bytes alone is not enough.
type Relocation struct {
	Address uint64 `json:"address"`
	Value   uint64 `json:"value"`
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
	for idx := range i.Sections {
		section := &i.Sections[idx]
		if address >= section.VirtualAddress && address-section.VirtualAddress < section.MemorySize {
			return section, true
		}
	}
	return nil, false
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

// SymbolByName returns the first symbol with this name.
func (i *Image) SymbolByName(name string) (symbol.Symbol, bool) {
	if i == nil || name == "" {
		return symbol.Symbol{}, false
	}
	for _, entry := range i.Symbols {
		if entry.Name == name {
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
