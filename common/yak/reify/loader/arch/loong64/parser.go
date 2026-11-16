// Package loong64 parses LoongArch 64-bit machine code into LowIR.
package loong64

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Parser parses LoongArch 64-bit machine code.
type Parser struct{}

var (
	_ core.Parser             = Parser{}
	_ core.Disassembler       = Parser{}
	_ core.BoundaryDiscoverer = Parser{}
)

// registers describes LoongArch register naming and the argument registers.
func registers() core.RegisterModel {
	return core.RegisterModel{
		Naming:    core.RegisterNamingPlain,
		Width:     64,
		Arguments: []string{"r4", "r5", "r6", "r7", "r8", "r9", "r10", "r11"},
		Return:    "r4",
	}
}

// frame describes the LoongArch call frame. r3 is the stack pointer, r22 the
// frame pointer and r1 the return address register.
func frame() core.FrameModel {
	return core.FrameModel{
		StackPointer:  "r3",
		FramePointer:  "r22",
		ReturnAddress: "r1",
		CalleeSaved:   []string{"r23", "r24", "r25", "r26", "r27", "r28", "r29", "r30", "r31", "r22"},
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
