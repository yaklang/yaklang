package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// MIPS64 parses 64-bit MIPS machine code.
type MIPS64 struct{}

var _ core.Parser = MIPS64{}

func (MIPS64) Kind() core.Architecture { return core.ArchMIPS64 }
func (MIPS64) Name() string            { return "mips64" }
func (MIPS64) Available() bool         { return false }
func (MIPS64) Check(target core.Target) error {
	return checkTarget(target, core.ArchMIPS64, rule{bits: 64})
}
func (MIPS64) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchMIPS64)
}
