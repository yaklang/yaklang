package asm2lowir

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

type stubParser struct {
	requests int
}

func (p *stubParser) Kind() core.Architecture { return core.ArchAMD64 }
func (p *stubParser) Name() string            { return "stub" }
func (p *stubParser) Available() bool         { return true }
func (p *stubParser) Check(core.Target) error { return nil }
func (p *stubParser) Parse(_ context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	p.requests++
	return &core.ParseResult{IR: &core.LowIR{
		Target: request.Target,
		Functions: []core.Function{{
			Address: request.Address,
			Blocks:  []core.Block{{ID: 1, Address: request.Address}},
		}},
	}}, nil
}
func (p *stubParser) DiscoverBoundaries(*core.ByteReader, core.BoundaryHints, *core.Image) (core.BoundaryResult, error) {
	return core.BoundaryResult{
		Functions: []core.FunctionBoundary{
			{Address: 0x1000, Size: 4, Name: "main", Source: core.BoundarySymbol},
			{Address: 0x1004, Size: 4, Source: core.BoundaryCall},
		},
		Stats: core.BoundaryStats{Symbol: 1, Call: 1},
	}, nil
}

func testArtifact(t *testing.T, parser core.Parser) *core.Artifact {
	t.Helper()
	reader, err := core.NewByteReader("fixture", strings.NewReader("0123456789abcdef"), 16)
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{
		Origin: core.Origin{Name: "fixture", Size: 16},
		Kind:   core.KindNative,
		Format: core.FormatELF,
		Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Image: &core.Image{
			Sections: []core.Section{{Name: ".text", FileOffset: 0, FileSize: 16, VirtualAddress: 0x1000, MemorySize: 16}},
			Symbols:  []sym.Symbol{{Name: "main", Address: 0x1000, Size: 4, Kind: sym.SymbolFunction}},
		},
		Hints:  &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Offset: 0, Size: 16}}},
		Reader: reader,
		Parser: parser,
	}
}

func TestParseSplitsFunctionsAndNamesThem(t *testing.T) {
	parser := &stubParser{}
	artifact := testArtifact(t, parser)
	result, err := Parse(context.Background(), artifact, WithAll())
	if err != nil {
		t.Fatal(err)
	}
	if parser.requests != 2 {
		t.Fatalf("parser calls = %d, want one per boundary", parser.requests)
	}
	if len(result.IR.Functions) != 2 {
		t.Fatalf("functions = %d", len(result.IR.Functions))
	}
	if result.IR.Functions[0].Name != "main" || result.IR.Functions[0].Boundary != core.BoundarySymbol {
		t.Fatalf("first function = %+v", result.IR.Functions[0])
	}
	if result.BoundaryStats.Symbol != 1 {
		t.Fatalf("stats = %+v", result.BoundaryStats)
	}
}

func TestParseRejectsMissingSelection(t *testing.T) {
	artifact := testArtifact(t, &stubParser{})
	if _, err := Parse(context.Background(), artifact); err == nil {
		t.Fatal("missing selection was accepted")
	}
}

// thumbSymbolParser reports one Thumb function whose symbol value keeps the
// marker bit set. A boundary discoverer aligns it and records the mode.
type thumbSymbolParser struct {
	stubParser
	address      uint64
	size         int64
	mode         string
	disasmAddr   uint64
	disasmSize   int64
	disasmMode   string
	discovered   []core.FunctionBoundary
	disasmResult *core.Disassembly
}

func (p *thumbSymbolParser) DiscoverBoundaries(*core.ByteReader, core.BoundaryHints, *core.Image) (core.BoundaryResult, error) {
	if len(p.discovered) > 0 {
		return core.BoundaryResult{Functions: p.discovered}, nil
	}
	return core.BoundaryResult{Functions: []core.FunctionBoundary{{
		Address: 0x1000, Size: 4, Name: "thumb_func", Source: core.BoundarySymbol, Mode: "thumb",
	}}}, nil
}

func (p *thumbSymbolParser) Parse(_ context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	p.address, p.size, p.mode = request.Address, request.Reader.Size(), request.Target.Mode
	return p.stubParser.Parse(context.Background(), request)
}

func (p *thumbSymbolParser) Disassemble(_ context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	p.disasmAddr, p.disasmSize, p.disasmMode = request.Address, request.Reader.Size(), request.Target.Mode
	if p.disasmResult != nil {
		return p.disasmResult, nil
	}
	return &core.Disassembly{Text: "0x1000  07 20  movs r0, #7\n", Instructions: 1}, nil
}

// noDiscoverThumbParser implements Parser and Disassembler without BoundaryDiscoverer
// so symbol-fallback paths are exercised directly.
type noDiscoverThumbParser struct {
	address    uint64
	size       int64
	mode       string
	disasmAddr uint64
	disasmSize int64
	disasmMode string
}

func (p *noDiscoverThumbParser) Kind() core.Architecture { return core.ArchARM }
func (p *noDiscoverThumbParser) Name() string            { return "no-discover-thumb" }
func (p *noDiscoverThumbParser) Available() bool         { return true }
func (p *noDiscoverThumbParser) Check(core.Target) error { return nil }
func (p *noDiscoverThumbParser) Parse(_ context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	p.address, p.size, p.mode = request.Address, request.Reader.Size(), request.Target.Mode
	return &core.ParseResult{IR: &core.LowIR{
		Target: request.Target,
		Functions: []core.Function{{
			Address: request.Address,
			Blocks:  []core.Block{{ID: 1, Address: request.Address}},
		}},
	}}, nil
}
func (p *noDiscoverThumbParser) Disassemble(_ context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	p.disasmAddr, p.disasmSize, p.disasmMode = request.Address, request.Reader.Size(), request.Target.Mode
	return &core.Disassembly{Text: "0x1000  07 20  movs r0, #7\n", Instructions: 1}, nil
}

// TestFunctionSelectionUsesTheDiscoveredBoundary checks that decoding one function
// by name starts where the boundary discoverer said, not at the raw symbol value.
// On ARM a Thumb symbol is stored with the low bit set, and decoding from there
// lands half an instruction early.
func TestFunctionSelectionUsesTheDiscoveredBoundary(t *testing.T) {
	parser := &thumbSymbolParser{}
	artifact := testArtifact(t, parser)
	artifact.Target = core.Target{OS: core.OSLinux, Architecture: core.ArchARM, Bits: 32, ByteOrder: core.LittleEndian}
	artifact.Image.Symbols = []sym.Symbol{{Name: "thumb_func", Address: 0x1001, Size: 4, Kind: sym.SymbolFunction}}
	artifact.Hints = &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 16}}}

	if _, err := Parse(context.Background(), artifact, WithFunction("thumb_func")); err != nil {
		t.Fatal(err)
	}
	if parser.address != 0x1000 {
		t.Fatalf("parsed address = %#x, want 0x1000", parser.address)
	}
	if parser.mode != "thumb" {
		t.Fatalf("parsed mode = %q, want thumb", parser.mode)
	}
}

func TestDisassembleHonorsFunctionMode(t *testing.T) {
	for _, tc := range []struct {
		name         string
		defaultMode  string
		boundaryMode string
	}{
		{"thumb function in arm image", "arm", "thumb"},
		{"arm function in thumb image", "thumb", "arm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := &thumbSymbolParser{
				discovered: []core.FunctionBoundary{{
					Address: 0x1000, Size: 4, Name: "target_fn", Source: core.BoundarySymbol, Mode: tc.boundaryMode,
				}},
			}
			artifact := testArtifact(t, parser)
			artifact.Target = core.Target{OS: core.OSLinux, Architecture: core.ArchARM, Bits: 32, ByteOrder: core.LittleEndian, Mode: tc.defaultMode}
			artifact.Image.Symbols = []sym.Symbol{{Name: "target_fn", Address: 0x1000, Size: 4, Kind: sym.SymbolFunction}}
			artifact.Hints = &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 16}}}

			if _, err := Disassemble(context.Background(), artifact, WithFunction("target_fn")); err != nil {
				t.Fatal(err)
			}
			if parser.disasmMode != tc.boundaryMode {
				t.Fatalf("disassembled mode = %q, want %q", parser.disasmMode, tc.boundaryMode)
			}
			if parser.disasmAddr != 0x1000 {
				t.Fatalf("disassembled address = %#x, want 0x1000", parser.disasmAddr)
			}
		})
	}
}

func TestFunctionSelectionAliasPreservesThumbMode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		symbolSize uint64
	}{
		{"explicit size", 4},
		{"inferred size", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser := &thumbSymbolParser{}
			artifact := testArtifact(t, parser)
			artifact.Target = core.Target{OS: core.OSLinux, Architecture: core.ArchARM, Bits: 32, ByteOrder: core.LittleEndian, Mode: "arm"}
			artifact.Image.Symbols = []sym.Symbol{
				{Name: "thumb_func", Address: 0x1001, Size: tc.symbolSize, Kind: sym.SymbolFunction},
				{Name: "thumb_alias", Address: 0x1001, Size: tc.symbolSize, Kind: sym.SymbolFunction},
				{Name: "next_func", Address: 0x1005, Size: 4, Kind: sym.SymbolFunction},
			}
			artifact.Hints = &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 16}}}

			parsed, err := Parse(context.Background(), artifact, WithFunction("thumb_alias"))
			if err != nil {
				t.Fatal(err)
			}
			if parser.address != 0x1000 || parser.size != 4 || parser.mode != "thumb" {
				t.Fatalf("Parse got addr=%#x size=%d mode=%q, want 0x1000/4/thumb", parser.address, parser.size, parser.mode)
			}
			if parsed.IR.Functions[0].Name != "thumb_alias" || parsed.IR.Functions[0].Size != 4 {
				t.Fatalf("unexpected function = %+v", parsed.IR.Functions[0])
			}

			if _, err := Disassemble(context.Background(), artifact, WithFunction("thumb_alias")); err != nil {
				t.Fatal(err)
			}
			if parser.disasmAddr != 0x1000 || parser.disasmSize != 4 || parser.disasmMode != "thumb" {
				t.Fatalf("Disassemble got addr=%#x size=%d mode=%q, want 0x1000/4/thumb", parser.disasmAddr, parser.disasmSize, parser.disasmMode)
			}
		})

		t.Run("no discoverer fallback/"+tc.name, func(t *testing.T) {
			parser := &noDiscoverThumbParser{}
			artifact := testArtifact(t, parser)
			artifact.Target = core.Target{OS: core.OSLinux, Architecture: core.ArchARM, Bits: 32, ByteOrder: core.LittleEndian, Mode: "arm"}
			artifact.Image.Symbols = []sym.Symbol{
				{Name: "thumb_func", Address: 0x1001, Size: tc.symbolSize, Kind: sym.SymbolFunction},
				{Name: "thumb_alias", Address: 0x1001, Size: tc.symbolSize, Kind: sym.SymbolFunction},
				{Name: "next_func", Address: 0x1005, Size: 4, Kind: sym.SymbolFunction},
			}
			artifact.Hints = &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 16}}}

			if _, err := Parse(context.Background(), artifact, WithFunction("thumb_alias")); err != nil {
				t.Fatal(err)
			}
			if parser.address != 0x1000 || parser.size != 4 || parser.mode != "thumb" {
				t.Fatalf("fallback Parse got addr=%#x size=%d mode=%q, want 0x1000/4/thumb", parser.address, parser.size, parser.mode)
			}
			if _, err := Disassemble(context.Background(), artifact, WithFunction("thumb_alias")); err != nil {
				t.Fatal(err)
			}
			if parser.disasmAddr != 0x1000 || parser.disasmSize != 4 || parser.disasmMode != "thumb" {
				t.Fatalf("fallback Disassemble got addr=%#x size=%d mode=%q, want 0x1000/4/thumb", parser.disasmAddr, parser.disasmSize, parser.disasmMode)
			}
		})
	}
}

func TestCodeAddressOnlyMasksARM(t *testing.T) {
	for _, tc := range []struct {
		arch core.Architecture
		addr uint64
		want uint64
	}{
		{core.ArchARM, 0x31, 0x30},
		{core.ArchAMD64, 0x31, 0x31},
		{core.Arch386, 0x31, 0x31},
		{core.ArchARM64, 0x31, 0x31},
	} {
		if got := codeAddress(tc.arch, tc.addr); got != tc.want {
			t.Fatalf("codeAddress(%s, %#x) = %#x, want %#x", tc.arch, tc.addr, got, tc.want)
		}
	}
}
