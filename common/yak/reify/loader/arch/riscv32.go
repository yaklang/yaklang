package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// RISCV32 parses 32-bit RISC-V machine code.
type RISCV32 struct{}

var _ core.Parser = RISCV32{}

func (RISCV32) Kind() core.Architecture { return core.ArchRISCV32 }
func (RISCV32) Name() string            { return "riscv32" }
func (RISCV32) Available() bool         { return false }
func (RISCV32) Check(target core.Target) error {
	return checkTarget(target, core.ArchRISCV32, rule{bits: 32})
}
func (RISCV32) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchRISCV32)
}
