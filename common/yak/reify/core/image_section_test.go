package core

import (
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestSectionAtPrefersFileBackedOverlap(t *testing.T) {
	// A TLS .tbss can share virtual addresses with the data that follows it.
	// Its bytes are not in the file, so a read must not pick it first.
	image := &Image{Sections: []Section{
		{Name: ".tbss", VirtualAddress: 0x2000, MemorySize: 0x40, FileSize: 0},
		{Name: ".data.rel.ro", VirtualAddress: 0x2020, MemorySize: 0x100, FileSize: 0x100, FileOffset: 0x400},
	}}
	section, ok := image.SectionAt(0x2030)
	if !ok || section.Name != ".data.rel.ro" {
		t.Fatalf("SectionAt = %+v, %v", section, ok)
	}
	if section, ok := image.SectionAt(0x2010); !ok || section.Name != ".tbss" {
		t.Fatalf("zero-fill section lost: %+v, %v", section, ok)
	}
	if _, ok := image.SectionAt(0x3000); ok {
		t.Fatal("unmapped address matched")
	}
}

func TestImageCompactIndexedLookupsAndInvalidation(t *testing.T) {
	image := &Image{
		Sections: []Section{
			{Name: ".text", VirtualAddress: 0x1000, MemorySize: 0x1000},
		},
		Relocations: []Relocation{
			{Address: 0x1050, Value: 0x1200, Symbol: "helper"},
		},
	}
	image.SetSymbols([]symbol.Symbol{
		{Name: "imp_fn", Address: 0x1000, Size: 0x200, Imported: true, Kind: symbol.SymbolFunction},
		{Name: "outer", Mangled: "_Z5outerv", Address: 0x1100, Size: 0x100, Kind: symbol.SymbolFunction},
		{Name: "inner", Address: 0x1140, Size: 0x20, Kind: symbol.SymbolFunction},
		{Name: "alias_a", Address: 0x1200, Kind: symbol.SymbolFunction},
		{Name: "alias_b", Address: 0x1200, Kind: symbol.SymbolFunction},
		{Name: "unsized", Address: 0x1300, Kind: symbol.SymbolFunction},
		{Name: "next_fn", Address: 0x1380, Kind: symbol.SymbolFunction},
	})

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, ok := image.SymbolAt(0x1100); !ok || got.Name != "outer" {
				t.Errorf("SymbolAt(0x1100) = %+v, %v", got, ok)
			}
			if got, ok := image.SymbolByName("_Z5outerv"); !ok || got.Name != "outer" {
				t.Errorf("SymbolByName(mangled) = %+v, %v", got, ok)
			}
			if got := image.SymbolsAt(0x1200); len(got) != 2 || got[0].Name != "alias_a" || got[1].Name != "alias_b" {
				t.Errorf("SymbolsAt(0x1200) = %+v", got)
			}
			if got := image.SymbolsAt(0x1100); len(got) != 1 || got[0].Name != "outer" {
				t.Errorf("SymbolsAt(0x1100) = %+v", got)
			}
			if got, ok := image.SymbolAt(0x1148); !ok || got.Name != "outer" {
				t.Errorf("SymbolAt(0x1148) = %+v, %v", got, ok)
			}
			if got, ok := image.SymbolAt(0x1010); ok {
				t.Errorf("SymbolAt imported interior returned %+v", got)
			}
			if got := image.InferredSymbolSize(symbol.Symbol{Address: 0x1300}); got != 0x80 {
				t.Errorf("InferredSymbolSize(0x1300) = 0x%x, want 0x80", got)
			}
			if rel, ok := image.RelocationAt(0x1050); !ok || rel.Symbol != "helper" {
				t.Errorf("RelocationAt = %+v, %v", rel, ok)
			}
		}()
	}
	wg.Wait()

	image.SetSymbols([]symbol.Symbol{
		{Name: "fresh", Address: 0x1400, Size: 0x40, Kind: symbol.SymbolFunction},
	})
	if _, ok := image.SymbolByName("_Z5outerv"); ok {
		t.Fatal("stale mangled symbol survived SetSymbols")
	}
	if got := image.SymbolsAt(0x1200); len(got) != 0 {
		t.Fatalf("stale duplicate symbol survived SetSymbols: %+v", got)
	}
	if got, ok := image.SymbolAt(0x1400); !ok || got.Name != "fresh" {
		t.Fatalf("SymbolAt(0x1400) after SetSymbols = %+v, %v", got, ok)
	}
}
