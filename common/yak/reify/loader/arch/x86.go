package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// X86 parses 32-bit x86 machine code.
type X86 struct{}

var _ core.Parser = X86{}

func (X86) Kind() core.Architecture { return core.Arch386 }
func (X86) Name() string            { return "386" }
func (X86) Available() bool         { return false }
func (X86) Check(target core.Target) error {
	return checkTarget(target, core.Arch386, rule{bits: 32})
}
func (X86) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(core.Arch386)
}
