// Package symbol defines artifact symbols and the built-in C signature table.
// It has no dependency on the core package, so core types can embed symbols.
package symbol

import "github.com/yaklang/yaklang/common/yak/reify/core/symbol/funcdefs"

// SymbolKind classifies a named address from a file-format symbol table.
type SymbolKind string

const (
	SymbolFunction SymbolKind = "function"
	SymbolObject   SymbolKind = "object"
	SymbolUnknown  SymbolKind = "unknown"
)

// Symbol source values record which structure supplied a name. An empty source
// means the container's own symbol table.
const (
	SourcePCLNTab      = "pclntab"
	SourceRustDemangle = "rust-demangle"
)

// Convention records which calling convention a function follows. An empty
// value means the language layer's convention, and a language without an
// override keeps the platform convention. A function the language does not own,
// such as a Go .abi0 wrapper or a cgo function, is marked explicitly.
type Convention string

const (
	// ConventionPlatform marks a function that follows the platform ABI even
	// inside an image whose language has its own convention.
	ConventionPlatform Convention = "platform"
)

// Symbol is one named address from a file-format symbol or relocation table.
// Imported symbols may have Address zero when the format only names them.
//
// Name is the name a consumer should display. Mangled keeps the name exactly as
// the image stores it when a language layer rewrote Name: a Rust symbol is
// stored as _RNv... and displayed as crate::func. A lookup accepts either form.
// Source names where the symbol came from: an empty value means the container's
// own symbol table, and "pclntab" means it was recovered from Go metadata.
type Symbol struct {
	Name       string     `json:"name"`
	Mangled    string     `json:"mangled,omitempty"`
	Address    uint64     `json:"address,omitempty"`
	Size       uint64     `json:"size,omitempty"`
	Kind       SymbolKind `json:"kind"`
	Imported   bool       `json:"imported,omitempty"`
	Library    string     `json:"library,omitempty"`
	Source     string     `json:"source,omitempty"`
	Convention Convention `json:"convention,omitempty"`
}

// At returns the symbol at address. An exact address match wins; a symbol with
// a size also matches addresses inside its range.
func At(symbols []Symbol, address uint64) (Symbol, bool) {
	for index := range symbols {
		item := &symbols[index]
		if item.Address == 0 {
			continue
		}
		if item.Address == address {
			return *item, true
		}
	}
	for index := range symbols {
		item := &symbols[index]
		if item.Imported || item.Size == 0 || item.Address == 0 || address < item.Address {
			continue
		}
		if address-item.Address < item.Size {
			return *item, true
		}
	}
	return Symbol{}, false
}

// Definition is one known C function signature.
type Definition = funcdefs.Definition

// Type and Param mirror the C signature types.
type (
	Type  = funcdefs.Type
	Param = funcdefs.Param
	Kind  = funcdefs.Kind
)

// C type kinds exposed through the unified symbol interface.
const (
	Void    = funcdefs.Void
	Int     = funcdefs.Int
	UInt    = funcdefs.UInt
	Size    = funcdefs.Size
	Pointer = funcdefs.Pointer
	String  = funcdefs.String
	Struct  = funcdefs.Struct
	Float   = funcdefs.Float
)

// Lookup returns a known C function signature.
func Lookup(name string) (Definition, bool) { return funcdefs.Lookup(name) }

// All returns every known C function signature.
func All() []Definition { return funcdefs.All() }
