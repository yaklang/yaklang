// Package s390x parses IBM z/Architecture machine code into LowIR. The decoder
// and lifter live in loader/arch/internal/s390x.
package s390x

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	internal "github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/s390x"
)

// Parser parses IBM z/Architecture machine code.
type Parser struct{}

var _ core.Parser = Parser{}
var _ core.Disassembler = Parser{}
var _ core.BoundaryDiscoverer = Parser{}

func profile() internal.Profile { return internal.Profile64() }

func (Parser) Kind() core.Architecture { return core.ArchS390X }
func (Parser) Name() string            { return "s390x" }
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
