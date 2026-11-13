package core

import "github.com/yaklang/yaklang/common/yak/reify/core/symbol"

// Section preserves header facts. FileSize is zero for zero-filled sections.
// FileOffset is relative to Origin; VirtualAddress is an image address before
// runtime relocation. Flags retain their file-format-specific meaning.
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
	ObjectType string          `json:"object_type"`
	Base       uint64          `json:"image_base,omitempty"`
	Entry      *uint64         `json:"entry_point,omitempty"`
	Sections   []Section       `json:"sections"`
	Symbols    []symbol.Symbol `json:"symbols,omitempty"`
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

// SymbolAt returns the symbol at address.
func (i *Image) SymbolAt(address uint64) (symbol.Symbol, bool) {
	if i == nil {
		return symbol.Symbol{}, false
	}
	return symbol.At(i.Symbols, address)
}
