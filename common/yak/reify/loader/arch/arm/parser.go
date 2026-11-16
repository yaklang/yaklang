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

// registers describes AAPCS register naming and the argument registers.
func registers() core.RegisterModel {
	return core.RegisterModel{
		Naming:     core.RegisterNamingPlain,
		Width:      32,
		Flags:      "cpsr",
		FlagsWidth: 32,
		Arguments:  []string{"r0", "r1", "r2", "r3"},
		Return:     "r0",
	}
}

// frame describes the AAPCS call frame. r11 is the frame pointer and r14 the
// link register.
func frame() core.FrameModel {
	return core.FrameModel{
		StackPointer:  "sp",
		FramePointer:  "r11",
		ReturnAddress: "lr",
		Flags:         "cpsr",
		CalleeSaved:   []string{"r4", "r5", "r6", "r7", "r8", "r9", "r10", "r11"},
		StackGrowth:   -1,
	}
}

// Facts describes the call frame, the register naming and the stack guard of
// this architecture.
func (Parser) Facts() core.ArchFacts {
	return core.ArchFacts{
		Frame:     frame(),
		Registers: registers(),
		Canary:    core.CanaryModel{},
	}
}

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
	// The ARM encoding comes from golang.org/x/arch/arm/armasm; the Thumb and
	// Thumb-2 encodings are decoded locally. Any other mode is rejected rather
	// than silently misdecoded.
	switch target.Mode {
	case "", "arm", "thumb", "thumb2":
	default:
		return fmt.Errorf("arm decoder supports arm, thumb and thumb2 modes, got %q", target.Mode)
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
	name := op.String()
	stripped := name
	for _, code := range conditionNames {
		stripped = strings.TrimSuffix(stripped, "."+strings.ToUpper(code))
	}
	flagSetting := strings.HasSuffix(stripped, ".S")
	if stripped == name && !flagSetting {
		// The opcode has no condition or S variant, for example DMB or NOP.
		return op
	}
	base := op &^ 15
	if flagSetting {
		base -= 16
	}
	return base + 14
}
