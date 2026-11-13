package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// PPC parses 32-bit PowerPC machine code.
type PPC struct{}

var _ core.Parser = PPC{}

func (PPC) Kind() core.Architecture { return core.ArchPPC }
func (PPC) Name() string            { return "ppc" }
func (PPC) Available() bool         { return false }
func (PPC) Check(target core.Target) error {
	return checkTarget(target, core.ArchPPC, rule{bits: 32})
}
func (PPC) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchPPC)
}
