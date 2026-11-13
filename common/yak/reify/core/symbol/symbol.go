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
