// Package arm64 parses AArch64 machine code into LowIR.
package arm64

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Parser parses 64-bit ARM machine code.
type Parser struct{}

var (
	_ core.Parser       = Parser{}
	_ core.Disassembler = Parser{}
)

// registers describes AAPCS64 register naming and the argument registers.
func registers() core.RegisterModel {
	return core.RegisterModel{
		Naming:     core.RegisterNamingAArch64,
		Width:      64,
		Flags:      "nzcv",
		FlagsWidth: 64,
		Arguments:  []string{"x0", "x1", "x2", "x3", "x4", "x5", "x6", "x7"},
		Return:     "x0",
	}
}

// frame describes the AAPCS64 call frame. x29 is the frame pointer and x30 the
// link register.
func frame() core.FrameModel {
	return core.FrameModel{
		StackPointer:  "sp",
		FramePointer:  "x29",
		ReturnAddress: "x30",
		Flags:         "nzcv",
		CalleeSaved:   []string{"x19", "x20", "x21", "x22", "x23", "x24", "x25", "x26", "x27", "x28", "x29"},
		StackGrowth:   -1,
	}
}

// Facts describes the call frame, the register naming and the stack guard of
// this architecture.
func (Parser) Facts() core.ArchFacts {
	return core.ArchFacts{
		Frame:     frame(),
		Registers: registers(),
		Canary:    core.CanaryModel{Kind: core.CanaryTLS, Register: "tpidr_el0", Offset: 0x28},
	}
}

func (Parser) Kind() core.Architecture { return core.ArchARM64 }
func (Parser) Name() string            { return "arm64" }
func (Parser) Available() bool         { return true }
func (Parser) Check(target core.Target) error {
	if target.Architecture != core.ArchARM64 {
		return fmt.Errorf("wanted architecture %s, got %s", core.ArchARM64, target.Architecture)
	}
	if target.Bits != 64 {
		return fmt.Errorf("arm64 requires 64-bit addresses, got %d", target.Bits)
	}
	if target.Mode != "" && target.Mode != "arm64" {
		return fmt.Errorf("arm64 does not support execution mode %q", target.Mode)
	}
	return nil
}

func (Parser) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	return parse(ctx, request)
}

const maxARM64Diagnostics = 64

type diagnostics struct {
	items   []core.Diagnostic
	dropped int
}

func (d *diagnostics) add(code, message string) {
	if len(d.items) < maxARM64Diagnostics {
		d.items = append(d.items, core.Diagnostic{Code: code, Message: message})
		return
	}
	d.dropped++
}

func (d *diagnostics) finish() []core.Diagnostic {
	if d.dropped == 0 {
		return d.items
	}
	return append(d.items, core.Diagnostic{Code: "diagnostics-truncated", Message: "further arm64 diagnostics were omitted"})
}
