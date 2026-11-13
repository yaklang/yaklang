package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// RISCV64 parses 64-bit RISC-V machine code.
type RISCV64 struct{}

var _ core.Parser = RISCV64{}

func (RISCV64) Kind() core.Architecture { return core.ArchRISCV64 }
func (RISCV64) Name() string            { return "riscv64" }
func (RISCV64) Available() bool         { return false }
func (RISCV64) Check(target core.Target) error {
	return checkTarget(target, core.ArchRISCV64, rule{bits: 64})
}
func (RISCV64) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchRISCV64)
}
