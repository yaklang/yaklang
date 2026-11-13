package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// ARM parses 32-bit ARM machine code in ARM or Thumb mode.
type ARM struct{}

var _ core.Parser = ARM{}

func (ARM) Kind() core.Architecture { return core.ArchARM }
func (ARM) Name() string            { return "arm" }
func (ARM) Available() bool         { return false }
func (ARM) Check(target core.Target) error {
	return checkTarget(target, core.ArchARM, rule{bits: 32, modes: []string{"arm", "thumb"}})
}
func (ARM) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchARM)
}
