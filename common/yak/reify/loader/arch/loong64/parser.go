// Package loong64 parses LoongArch 64-bit machine code into LowIR.
package loong64

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Parser parses LoongArch 64-bit machine code.
type Parser struct{}

var (
	_ core.Parser             = Parser{}
	_ core.Disassembler       = Parser{}
	_ core.BoundaryDiscoverer = Parser{}
)

func (Parser) Kind() core.Architecture { return architecture }
func (Parser) Name() string            { return architectureName }
func (Parser) Available() bool         { return true }

func (Parser) Check(target core.Target) error { return checkTarget(target) }

func (Parser) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	if err := checkTarget(request.Target); err != nil {
		return nil, err
	}
	return parse(ctx, request)
}

func (Parser) Disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	if err := checkTarget(request.Target); err != nil {
		return nil, err
	}
	return disassemble(ctx, request)
}

func (Parser) DiscoverBoundaries(reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	return discoverBoundaries(reader, hints, image)
}
