// Package x86_16 parses sixteen-bit x86 machine code into LowIR. The decoder and
// lifter are shared with the 386 and AMD64 parsers through
// loader/arch/internal/x86; only the profile differs.
package x86_16

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	internal "github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/x86"
)

// Parser parses sixteen-bit x86 machine code.
type Parser struct{}

var _ core.Parser = Parser{}
var _ core.Disassembler = Parser{}
var _ core.BoundaryDiscoverer = Parser{}

func profile() internal.Profile { return internal.Profile16() }

func (Parser) Kind() core.Architecture { return core.Arch8086 }
func (Parser) Name() string            { return "8086" }
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
