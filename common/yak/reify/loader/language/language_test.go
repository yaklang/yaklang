package language

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// metadataArtifact builds a minimal native artifact around one named section.
func metadataArtifact(t *testing.T, section string, data []byte) *core.Artifact {
	t.Helper()
	r, err := core.NewByteReader("metadata", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{Kind: core.KindNative, Format: core.FormatELF, Reader: r,
		Target: core.Target{Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Image:  &core.Image{Sections: []core.Section{{Name: section, FileSize: uint64(len(data)), MemorySize: uint64(len(data))}}},
		Hints:  &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 0x100}}}}
}

func TestCDefaultAndMalformedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, section string
		data          []byte
	}{
		{"plain C", ".text", []byte{0x90, 0xc3}},
		{"name is not proof", ".gopclntab", []byte("not a Go table")},
		{"truncated", ".gopclntab", []byte{0xf1, 0xff, 0xff, 0xff, 0, 0, 1, 8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := metadataArtifact(t, tc.section, tc.data)
			report := Inspect(a)
			if report.Language != core.LanguageC || report.Confidence != core.ConfidenceNone {
				t.Fatalf("default language = %+v", report)
			}
			if report.ABI != nil {
				t.Fatalf("C default must not add an ABI: %+v", report.ABI)
			}
			if report.Strings == nil || report.Strings.Layout != core.StringLayoutNulTerminated || report.Strings.Encoding != core.StringEncodingBytes {
				t.Fatalf("C string model = %+v", report.Strings)
			}
			if len(a.Image.Symbols) != 0 {
				t.Fatalf("default layer gained symbols: %+v", a.Image.Symbols)
			}
			if tc.section == ".gopclntab" && len(report.Notes) == 0 {
				t.Fatalf("rejected Go metadata lost its failure note: %+v", report)
			}
			if Inspect(a) != report {
				t.Fatal("language inspection is not idempotent")
			}
		})
	}
}

func TestRustNamesKeepIdentityAndRebuildIndex(t *testing.T) {
	a := metadataArtifact(t, ".text", []byte{0x90, 0xc3})
	raw := []string{"_ZN3foo3bar17h0123456789abcdefE", "_ZN3foo3bar17h1123456789abcdefE"}
	a.Image.Symbols = []symbol.Symbol{
		{Name: raw[0], Address: 0x1000, Kind: symbol.SymbolFunction},
		{Name: raw[1], Address: 0x1010, Kind: symbol.SymbolFunction},
	}
	_ = a.Image.SymbolsAt(0x1000)
	r := Inspect(a)
	if r.Language != core.LanguageRust || r.Confidence != core.ConfidenceCertain {
		t.Fatalf("report = %+v", r)
	}
	for i, address := range []uint64{0x1000, 0x1010} {
		s := a.Image.SymbolsAt(address)[0]
		if s.Mangled != raw[i] || s.Source != symbol.SourceRustDemangle {
			t.Fatalf("symbol = %+v", s)
		}
		if s.Name != "foo::bar@0x1000" && s.Name != "foo::bar@0x1010" {
			t.Fatalf("colliding name was not disambiguated: %+v", s)
		}
		if found, ok := a.Image.SymbolByName(raw[i]); !ok || found.Name != s.Name {
			t.Fatalf("encoded alias lookup = %+v, %v", found, ok)
		}
	}
	if a.Image.Symbols[0].Name == a.Image.Symbols[1].Name {
		t.Fatal("distinct Rust functions collapsed into one name")
	}
	if Inspect(a) != r {
		t.Fatal("language inspection is not idempotent")
	}
}

func TestRustCommentWithoutSymbolsIsOnlyLikely(t *testing.T) {
	a := metadataArtifact(t, ".comment", []byte("GCC version\x00rustc version 1.97.1\x00"))
	r := Inspect(a)
	if r.Language != core.LanguageRust || r.Confidence != core.ConfidenceLikely || len(a.Image.Symbols) != 0 {
		t.Fatalf("comment cannot recover stripped names: %+v", r)
	}
}

func TestMergeGoSymbolsKeepsExactSizeAndProvenance(t *testing.T) {
	a := metadataArtifact(t, ".text", nil)
	a.Image.Symbols = []symbol.Symbol{{Name: "main.Add", Address: 0x1000, Size: 7, Kind: symbol.SymbolFunction}}
	mergeSymbols(a, []symbol.Symbol{
		{Name: "main.Add", Address: 0x1000, Size: 32, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab},
		{Name: "main.main", Address: 0x1020, Size: 32, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab},
	})
	if len(a.Image.Symbols) != 2 || a.Image.Symbols[0].Size != 7 || a.Image.Symbols[0].Source != symbol.SourcePCLNTab {
		t.Fatalf("symbols = %+v", a.Image.Symbols)
	}
}

func TestMergeKeepsSameNameAtDifferentAddresses(t *testing.T) {
	a := metadataArtifact(t, ".text", nil)
	mergeSymbols(a, []symbol.Symbol{
		{Name: "runtime.duffcopy", Address: 0x1000, Size: 16, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab},
		{Name: "runtime.duffcopy", Address: 0x1100, Size: 32, Kind: symbol.SymbolFunction, Source: symbol.SourcePCLNTab},
	})
	if len(a.Image.Symbols) != 2 {
		t.Fatalf("distinct bodies with one name collapsed: %+v", a.Image.Symbols)
	}
	if a.Image.Symbols[0].Size != 16 || a.Image.Symbols[1].Size != 32 {
		t.Fatalf("sizes = %d, %d", a.Image.Symbols[0].Size, a.Image.Symbols[1].Size)
	}
}

func TestMergeRenamesEncodedSymbolInPlace(t *testing.T) {
	a := metadataArtifact(t, ".text", nil)
	a.Image.Symbols = []symbol.Symbol{{Name: "_RNvCs15kBYyAo9fc_7mycrate3add", Address: 0x1000, Size: 12, Kind: symbol.SymbolFunction}}
	Inspect(a)
	if len(a.Image.Symbols) != 1 {
		t.Fatalf("rename duplicated the symbol: %+v", a.Image.Symbols)
	}
	if s := a.Image.Symbols[0]; s.Name != "mycrate::add" || s.Mangled != "_RNvCs15kBYyAo9fc_7mycrate3add" {
		t.Fatalf("symbol = %+v", s)
	}
}

func TestEveryLanguageHasOneLayer(t *testing.T) {
	seen := map[core.Language]bool{}
	for _, layer := range languages {
		if seen[layer.Kind()] {
			t.Fatalf("duplicate layer for %s", layer.Kind())
		}
		seen[layer.Kind()] = true
	}
	for _, want := range []core.Language{core.LanguageC, core.LanguageGo, core.LanguageRust} {
		if !seen[want] {
			t.Fatalf("no layer for %s", want)
		}
		if languageFor(want) == nil || languageFor(want).Kind() != want {
			t.Fatalf("lookup failed for %s", want)
		}
	}
	if got := languageFor(core.LanguageUnknown); got.Kind() != core.LanguageC {
		t.Fatalf("unknown kind resolved to %s, want the default C layer", got.Kind())
	}
}
