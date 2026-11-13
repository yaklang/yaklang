package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// PPC64 parses 64-bit PowerPC machine code.
type PPC64 struct{}

var _ core.Parser = PPC64{}

func (PPC64) Kind() core.Architecture { return core.ArchPPC64 }
func (PPC64) Name() string            { return "ppc64" }
func (PPC64) Available() bool         { return false }
func (PPC64) Check(target core.Target) error {
	return checkTarget(target, core.ArchPPC64, rule{bits: 64})
}
func (PPC64) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.ArchPPC64)
}
