package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// ARM64 parses 64-bit ARM machine code.
type ARM64 struct{}

var _ core.Parser = ARM64{}

func (ARM64) Kind() core.Architecture { return core.ArchARM64 }
func (ARM64) Name() string            { return "arm64" }
func (ARM64) Available() bool         { return false }
func (ARM64) Check(target core.Target) error {
	return checkTarget(target, core.ArchARM64, rule{bits: 64})
}
func (ARM64) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchARM64)
}
