package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// MIPS parses 32-bit MIPS machine code.
type MIPS struct{}

var _ core.Parser = MIPS{}

func (MIPS) Kind() core.Architecture { return core.ArchMIPS }
func (MIPS) Name() string            { return "mips" }
func (MIPS) Available() bool         { return false }
func (MIPS) Check(target core.Target) error {
	return checkTarget(target, core.ArchMIPS, rule{bits: 32})
}
func (MIPS) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchMIPS)
}
