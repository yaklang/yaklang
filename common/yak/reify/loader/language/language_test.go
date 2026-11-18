package language

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestRustDemangle(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"_RNvCs15kBYyAo9fc_7mycrate3add", "mycrate::add"},
		{"__RNvCs15kBYyAo9fc_7mycrate3add", "mycrate::add"},
		{"_ZN3foo3bar17h0123456789abcdefE", "foo::bar"},
		{"_RINvMNtNtCs4NRVxsYgnAr_4core3fmt2rtNtB3_8Argument11new_displayxECs4FrXkKnA8Dq_10rust_hello", "<core::fmt::rt::Argument>::new_display::<i64>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DemangleRust(tc.name)
			if !ok || got != tc.want {
				t.Fatalf("DemangleRust = %q, %v; want %q", got, ok, tc.want)
			}
		})
	}
	for _, name := range []string{"main.Add", "main", "_ZN3foo3barEv", "_RNonsense", "_ZN3foo3bar17hzzzzzzzzzzzzzzzzE"} {
		if got, ok := DemangleRust(name); ok {
			t.Errorf("non-Rust or malformed name %q demangled to %q", name, got)
		}
	}
}

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

func TestLanguageUnknownAndMalformedMetadata(t *testing.T) {
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
			if got := Inspect(a); got.Language != core.LanguageUnknown {
				t.Fatalf("language = %+v", got)
			}
		})
	}
	data := make([]byte, 80)
	binary.LittleEndian.PutUint32(data, 0xfffffff1)
	data[6], data[7] = 1, 8
	binary.LittleEndian.PutUint64(data[8:], math.MaxUint64)
	a := metadataArtifact(t, ".gopclntab", data)
	if syms, err := (goSymbols{}).Recover(a); err == nil || len(syms) != 0 {
		t.Fatalf("unbounded count accepted: %d symbols, %v", len(syms), err)
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
		if s.Mangled != raw[i] || !strings.HasPrefix(s.Name, "foo::bar@0x") {
			t.Fatalf("symbol = %+v", s)
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

func TestGoVersionAndRegisterModels(t *testing.T) {
	for _, tc := range []struct {
		version string
		minor   int
	}{{"", 0}, {"unknown", 0}, {"go1.16", 16}, {"go1.27.4-X:nodwarf5", 27}, {"go1.18rc1", 18}} {
		if got := goMinor(tc.version); got != tc.minor {
			t.Fatalf("goMinor(%q) = %d", tc.version, got)
		}
	}
	amd64 := goRegisters(core.Target{Architecture: core.ArchAMD64})
	arm64 := goRegisters(core.Target{Architecture: core.ArchARM64})
	if len(amd64.Arguments) != 9 || amd64.Arguments[0] != "rax" || amd64.Arguments[1] != "rbx" || len(arm64.Arguments) != 16 {
		t.Fatalf("Go integer register models: %+v %+v", amd64, arm64)
	}
	if goRegisters(core.Target{Architecture: core.Arch386}) != nil {
		t.Fatal("unimplemented ABI was guessed")
	}
}

func FuzzGoPCLNTab(f *testing.F) {
	f.Add([]byte{0xf1, 0xff, 0xff, 0xff, 0, 0, 1, 8, 1, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		a := metadataArtifact(t, ".gopclntab", data)
		Inspect(a)
	})
}

func codeArtifact(t *testing.T, code []byte) *core.Artifact {
	t.Helper()
	r, err := core.NewByteReader("code", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{Kind: core.KindNative, Format: core.FormatELF, Reader: r,
		Target: core.Target{Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Image: &core.Image{Sections: []core.Section{{Name: ".text", FileSize: uint64(len(code)),
			MemorySize: uint64(len(code)), VirtualAddress: 0x1000, Flags: 0x6}}}}
}

func TestGoFuncSizeTrimsOnlyAlignmentFill(t *testing.T) {
	x86 := append([]byte{0x55, 0x48, 0x89, 0xe5, 0xc3}, bytes.Repeat([]byte{0xcc}, 11)...)
	while := append([]byte{0x31, 0xdb, 0x90}, bytes.Repeat([]byte{0x90}, 5)...)
	whole := bytes.Repeat([]byte{0xcc}, 16)
	arm := append([]byte{0xc0, 0x03, 0x5f, 0xd6}, []byte{0x1f, 0x20, 0x03, 0xd5, 0, 0, 0, 0}...)
	for _, tc := range []struct {
		name string
		code []byte
		arch core.Architecture
		want uint64
	}{
		{"amd64 int3 tail", x86, core.ArchAMD64, 5},
		{"amd64 keeps one byte", whole, core.ArchAMD64, 1},
		{"amd64 nop tail", while, core.ArchAMD64, 2},
		{"arm64 nop and zero tail", arm, core.ArchARM64, 4},
		{"unknown architecture is untouched", x86, core.ArchRISCV64, uint64(len(x86))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := codeArtifact(t, tc.code)
			a.Target.Architecture = tc.arch
			if got := goFuncSize(a, 0x1000, uint64(len(tc.code)), tc.arch); got != tc.want {
				t.Fatalf("goFuncSize = %d, want %d", got, tc.want)
			}
		})
	}
	// A range outside the section must keep the original size.
	a := codeArtifact(t, x86)
	if got := goFuncSize(a, 0x9000, 16, core.ArchAMD64); got != 16 {
		t.Fatalf("out-of-range size changed to %d", got)
	}
}
