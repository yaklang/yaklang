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
	address uint64
	mode    string
}

func (p *thumbSymbolParser) DiscoverBoundaries(*core.ByteReader, core.BoundaryHints, *core.Image) (core.BoundaryResult, error) {
	return core.BoundaryResult{Functions: []core.FunctionBoundary{{
		Address: 0x1000, Size: 4, Name: "thumb_func", Source: core.BoundarySymbol, Mode: "thumb",
	}}}, nil
}

func (p *thumbSymbolParser) Parse(_ context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	p.address, p.mode = request.Address, request.Target.Mode
	return p.stubParser.Parse(context.Background(), request)
}

// TestFunctionSelectionUsesTheDiscoveredBoundary checks that decoding one function
// by name starts where the boundary discoverer said, not at the raw symbol value.
// On ARM a Thumb symbol is stored with the low bit set, and decoding from there
// lands half an instruction early.
func TestFunctionSelectionUsesTheDiscoveredBoundary(t *testing.T) {
	parser := &thumbSymbolParser{}
	artifact := testArtifact(t, parser)
	artifact.Target = core.Target{OS: core.OSLinux, Architecture: core.ArchARM, Bits: 32, ByteOrder: core.LittleEndian}
	artifact.Image.Symbols = []sym.Symbol{
		{Name: "thumb_func", Address: 0x1001, Size: 4, Kind: sym.SymbolFunction},
		{Name: "thumb_alias", Address: 0x1001, Size: 4, Kind: sym.SymbolFunction},
	}
	artifact.Hints = &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 16}}}

	for _, name := range []string{"thumb_func", "thumb_alias"} {
		t.Run(name, func(t *testing.T) {
			result, err := Parse(context.Background(), artifact, WithFunction(name))
			if err != nil {
				t.Fatal(err)
			}
			if parser.address != 0x1000 || parser.mode != "thumb" {
				t.Fatalf("parsed address = %#x, mode = %q, want 0x1000 and thumb", parser.address, parser.mode)
			}
			if functions := result.IR.Functions; len(functions) != 1 || functions[0].Name != name || functions[0].Size != 4 {
				t.Fatalf("unexpected functions: %+v", functions)
			}
		})
	}
}
