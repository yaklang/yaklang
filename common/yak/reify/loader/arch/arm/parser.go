// Package arm parses 32-bit ARM machine code in ARM mode into LowIR.
package arm

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Parser parses 32-bit ARM machine code in ARM mode.
type Parser struct{}

var (
	_ core.Parser             = Parser{}
	_ core.Disassembler       = Parser{}
	_ core.BoundaryDiscoverer = Parser{}
)

func (Parser) Kind() core.Architecture { return core.ArchARM }
func (Parser) Name() string            { return "arm" }
func (Parser) Available() bool         { return true }

func (Parser) Check(target core.Target) error {
	if target.Architecture != core.ArchARM {
		return fmt.Errorf("wanted architecture %s, got %s", core.ArchARM, target.Architecture)
	}
	if target.Bits != 32 {
		return fmt.Errorf("arm requires 32-bit addresses, got %d", target.Bits)
	}
	// golang.org/x/arch/arm/armasm decodes ARM mode only; it returns
	// errMode for Thumb. Reject the mode instead of silently misdecoding.
	if target.Mode != "" && target.Mode != "arm" {
		return fmt.Errorf("arm decoder supports ARM mode only, got mode %q", target.Mode)
	}
	return nil
}

func (Parser) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	return parse(ctx, request)
}

const maxDiagnostics = 64

type diagnostics struct {
	items   []core.Diagnostic
	dropped int
}

func (d *diagnostics) add(code, message string) {
	if len(d.items) < maxDiagnostics {
		d.items = append(d.items, core.Diagnostic{Code: code, Message: message})
		return
	}
	d.dropped++
}

func (d *diagnostics) finish() []core.Diagnostic {
	if d.dropped == 0 {
		return d.items
	}
	return append(d.items, core.Diagnostic{
		Code:    "diagnostics-truncated",
		Message: "further arm diagnostics were omitted",
	})
}

// condition returns the condition suffix of one opcode. armasm packs the
// condition into the low four bits of Op, so the value is the ARM condition
// code: 0 EQ, 1 NE, ... 14 AL, 15 NV.
func condition(op armasm.Op) string {
	code := int(op & 15)
	if code < 0 || code >= len(conditionNames) {
		return "al"
	}
	return conditionNames[code]
}

var conditionNames = [...]string{
	"eq", "ne", "cs", "cc", "mi", "pl", "vs", "vc",
	"hi", "ls", "ge", "lt", "gt", "le", "al", "nv",
}

// baseOp returns the always-executed, flag-preserving spelling of an opcode.
// armasm numbers each base form as sixteen consecutive entries, one per ARM
// condition, and gives the flag-setting S variant its own block of sixteen. A
// name suffix decides which block the opcode belongs to.
func baseOp(op armasm.Op) armasm.Op {
	base := op &^ 15
	name := op.String()
	for _, condition := range conditionNames {
		name = strings.TrimSuffix(name, "."+strings.ToUpper(condition))
	}
	if strings.HasSuffix(name, ".S") {
		base -= 16
	}
	return base + 14
}
