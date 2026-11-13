package amd64

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Parser parses 64-bit x86 machine code into LowIR.
type Parser struct{}

var _ core.Parser = Parser{}
var _ core.Disassembler = Parser{}

func (Parser) Kind() core.Architecture { return core.ArchAMD64 }
func (Parser) Name() string            { return "amd64" }
func (Parser) Available() bool         { return true }
func (Parser) Check(target core.Target) error {
	if target.Architecture != core.ArchAMD64 {
		return fmt.Errorf("wanted architecture %s, got %s", core.ArchAMD64, target.Architecture)
	}
	if target.Bits != 64 {
		return fmt.Errorf("amd64 requires 64-bit addresses, got %d", target.Bits)
	}
	if target.Mode != "" {
		return fmt.Errorf("amd64 does not support execution mode %q", target.Mode)
	}
	return nil
}
func (Parser) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	return parse(ctx, request)
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
