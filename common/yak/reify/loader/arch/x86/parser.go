// Package x86 parses 32-bit x86 machine code into LowIR. The decoder and lifter
// are shared with the AMD64 parser through loader/arch/internal/x86.
package x86

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	internal "github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/x86"
)

// Parser parses 32-bit x86 machine code.
type Parser struct{}

var _ core.Parser = Parser{}
var _ core.Disassembler = Parser{}
var _ core.BoundaryDiscoverer = Parser{}

func profile() internal.Profile { return internal.Profile32() }

// Facts describes the call frame, the register naming and the stack guard of
// this architecture.
func (Parser) Facts() core.ArchFacts {
	profile := internal.Profile32()
	return core.ArchFacts{
		Frame:     profile.Frame(),
		Registers: profile.Registers(),
		Canary:    profile.Canary,
	}
}

func (Parser) Kind() core.Architecture { return core.Arch386 }
func (Parser) Name() string            { return "386" }
func (Parser) Available() bool         { return true }

func (Parser) Check(target core.Target) error { return profile().Check(target) }

func (Parser) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	return internal.Parse(ctx, profile(), request)
}

func (Parser) Disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	return internal.Disassemble(ctx, profile(), request)
}

func (Parser) DiscoverBoundaries(reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	return internal.DiscoverBoundaries(profile(), reader, hints, image)
}
