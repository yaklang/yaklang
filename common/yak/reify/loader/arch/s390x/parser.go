// Package s390x parses IBM z/Architecture machine code into LowIR.
package s390x

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Parser parses IBM z/Architecture machine code.
type Parser struct{}

var (
	_ core.Parser             = Parser{}
	_ core.Disassembler       = Parser{}
	_ core.BoundaryDiscoverer = Parser{}
)

// registers describes z/Architecture register naming and the argument registers.
func registers() core.RegisterModel {
	return core.RegisterModel{
		Naming:     core.RegisterNamingPlain,
		Width:      64,
		Flags:      "cc",
		FlagsWidth: 2,
		Arguments:  []string{"r2", "r3", "r4", "r5", "r6"},
		Return:     "r2",
	}
}

// frame describes the z/Architecture call frame. r15 is the stack pointer,
// r11 the frame pointer and r14 the return address register.
func frame() core.FrameModel {
	return core.FrameModel{
		StackPointer:  "r15",
		FramePointer:  "r11",
		ReturnAddress: "r14",
		Flags:         "cc",
		CalleeSaved:   []string{"r6", "r7", "r8", "r9", "r10", "r11", "r12", "r13"},
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

func (Parser) Kind() core.Architecture { return architecture }
func (Parser) Name() string            { return architectureName }
func (Parser) Available() bool         { return true }

func (Parser) Check(target core.Target) error { return checkTarget(target) }

func (Parser) Parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	if err := checkTarget(request.Target); err != nil {
		return nil, err
	}
	return parse(ctx, request)
}

func (Parser) Disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	if err := checkTarget(request.Target); err != nil {
		return nil, err
	}
	return disassemble(ctx, request)
}

func (Parser) DiscoverBoundaries(reader *core.ByteReader, hints core.BoundaryHints, image *core.Image) (core.BoundaryResult, error) {
	return discoverBoundaries(reader, hints, image)
}
