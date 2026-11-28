package core

import (
	"sort"
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

	// Lazy symbol and relocation indexes, built on first lookup.
	once         sync.Once
	byAddress    map[uint64][]symbol.Symbol
	byName       map[string]symbol.Symbol
	byMangled    map[string]symbol.Symbol
	sizedSymbols []indexedSymbol
	sizedMaxEnd  []uint64
	sortedAddrs  []uint64
	relocOnce    sync.Once
	relocByAddr  map[uint64]Relocation
}

type indexedSymbol struct {
	sym   symbol.Symbol
	order int
}

// SetSymbols replaces the symbol table during artifact construction. Call it
// before publishing the image to concurrent readers; it invalidates the lazy
// indexes when a metadata layer adds or renames symbols.
func (i *Image) SetSymbols(symbols []symbol.Symbol) {
	i.Symbols = symbols
	i.once = sync.Once{}
	i.byAddress = nil
	i.byName = nil
	i.byMangled = nil
	i.sizedSymbols = nil
	i.sizedMaxEnd = nil
	i.sortedAddrs = nil
}

func (i *Image) ensureSymbolIndex() {
	i.once.Do(func() {
		n := len(i.Symbols)
		i.byAddress = make(map[uint64][]symbol.Symbol, n)
		i.byName = make(map[string]symbol.Symbol, n)
		i.byMangled = make(map[string]symbol.Symbol)
		i.sizedSymbols = make([]indexedSymbol, 0, n)
		i.sortedAddrs = make([]uint64, 0, n)
		for idx, entry := range i.Symbols {
			i.byAddress[entry.Address] = append(i.byAddress[entry.Address], entry)
			if entry.Name != "" {
				if _, exists := i.byName[entry.Name]; !exists {
					i.byName[entry.Name] = entry
				}
			}
			if entry.Mangled != "" {
				if _, exists := i.byMangled[entry.Mangled]; !exists {
					i.byMangled[entry.Mangled] = entry
				}
			}
			if !entry.Imported && entry.Address != 0 {
				i.sortedAddrs = append(i.sortedAddrs, entry.Address)
				if entry.Size > 0 {
					i.sizedSymbols = append(i.sizedSymbols, indexedSymbol{sym: entry, order: idx})
				}
			}
		}
		sort.Slice(i.sizedSymbols, func(a, b int) bool {
			if i.sizedSymbols[a].sym.Address != i.sizedSymbols[b].sym.Address {
				return i.sizedSymbols[a].sym.Address < i.sizedSymbols[b].sym.Address
			}
			return i.sizedSymbols[a].order < i.sizedSymbols[b].order
		})
		i.sizedMaxEnd = make([]uint64, len(i.sizedSymbols))
		var maxEnd uint64
		for idx, item := range i.sizedSymbols {
			end := item.sym.Address + item.sym.Size
			if end < item.sym.Address {
				end = ^uint64(0)
			}
			if end > maxEnd {
				maxEnd = end
			}
			i.sizedMaxEnd[idx] = maxEnd
		}
		sort.Slice(i.sortedAddrs, func(a, b int) bool { return i.sortedAddrs[a] < i.sortedAddrs[b] })
		if len(i.sortedAddrs) > 1 {
			dedup := i.sortedAddrs[:1]
			for _, addr := range i.sortedAddrs[1:] {
				if addr != dedup[len(dedup)-1] {
					dedup = append(dedup, addr)
				}
			}
			i.sortedAddrs = dedup
		}
	})
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
	i.relocOnce.Do(func() {
		i.relocByAddr = make(map[uint64]Relocation, len(i.Relocations))
		for _, relocation := range i.Relocations {
			if _, exists := i.relocByAddr[relocation.Address]; !exists {
				i.relocByAddr[relocation.Address] = relocation
			}
		}
	})
	relocation, ok := i.relocByAddr[address]
	return relocation, ok
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
	i.ensureSymbolIndex()
	next := limit
	pos := sort.Search(len(i.sortedAddrs), func(idx int) bool { return i.sortedAddrs[idx] > entry.Address })
	if pos < len(i.sortedAddrs) {
		cand := i.sortedAddrs[pos]
		if cand-section.VirtualAddress < section.MemorySize && cand < next {
			next = cand
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
	i.ensureSymbolIndex()
	return i.byAddress[address]
}

// SymbolByName returns the first symbol with this name. A language layer keeps
// the compiler-encoded name in Mangled, so a caller may pass either the display
// name or the encoded one.
func (i *Image) SymbolByName(name string) (symbol.Symbol, bool) {
	if i == nil || name == "" {
		return symbol.Symbol{}, false
	}
	i.ensureSymbolIndex()
	if entry, ok := i.byName[name]; ok {
		return entry, true
	}
	if entry, ok := i.byMangled[name]; ok {
		return entry, true
	}
	return symbol.Symbol{}, false
}

// SymbolAt returns the symbol at address.
func (i *Image) SymbolAt(address uint64) (symbol.Symbol, bool) {
	if i == nil {
		return symbol.Symbol{}, false
	}
	i.ensureSymbolIndex()
	if address != 0 {
		if entries := i.byAddress[address]; len(entries) > 0 {
			return entries[0], true
		}
	}
	pos := sort.Search(len(i.sizedSymbols), func(idx int) bool { return i.sizedSymbols[idx].sym.Address > address })
	if pos == 0 {
		return symbol.Symbol{}, false
	}
	var best *indexedSymbol
	for idx := pos - 1; idx >= 0; idx-- {
		if i.sizedMaxEnd[idx] <= address {
			break
		}
		cand := &i.sizedSymbols[idx]
		if address-cand.sym.Address < cand.sym.Size {
			if best == nil || cand.order < best.order {
				best = cand
			}
		}
	}
	if best != nil {
		return best.sym, true
	}
	return symbol.Symbol{}, false
}
