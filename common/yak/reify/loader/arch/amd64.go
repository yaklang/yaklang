package arch

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// AMD64 parses 64-bit x86 machine code into LowIR.
type AMD64 struct{}

var _ core.Parser = AMD64{}

func (AMD64) Kind() core.Architecture { return core.ArchAMD64 }
func (AMD64) Name() string            { return "amd64" }
func (AMD64) Available() bool         { return true }
func (AMD64) Check(target core.Target) error {
	return checkTarget(target, core.ArchAMD64, rule{bits: 64, littleOnly: true})
}
func (AMD64) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	return parseAMD64(ctx, request)
}

const maxAMD64Diagnostics = 64

type amd64Diagnostics struct {
	items   []core.Diagnostic
	dropped int
}

func (d *amd64Diagnostics) add(code, message string) {
	if len(d.items) < maxAMD64Diagnostics {
		d.items = append(d.items, core.Diagnostic{Code: code, Message: message})
		return
	}
	d.dropped++
}

func (d *amd64Diagnostics) finish() []core.Diagnostic {
	if d.dropped == 0 {
		return d.items
	}
	return append(d.items, core.Diagnostic{
		Code:    "diagnostics-truncated",
		Message: "further amd64 diagnostics were omitted",
	})
}
