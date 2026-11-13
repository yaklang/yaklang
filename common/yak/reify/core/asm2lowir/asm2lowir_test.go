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
