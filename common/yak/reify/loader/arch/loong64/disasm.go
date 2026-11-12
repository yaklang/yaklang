package loong64

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/loong64/loong64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("%s: empty code region", architectureName)
	}
	diagnostics := &diagnostics{}
	decoded, err := decodeInstructions(ctx, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("%s: no instruction decoded at 0x%x", architectureName, request.Address)
	}
	end := request.Address + uint64(request.Reader.Size())
	functions, blockIDs := buildFunctions(decoded, request.Address, end, diagnostics)
	_ = liftFunctions(request.Target, functions, blockIDs, diagnostics)
	return &core.Disassembly{
		Text:         renderAssembly(decoded),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

// renderAssembly prints one instruction per line.
func renderAssembly(decoded []instruction) string {
	var text strings.Builder
	for _, ins := range decoded {
		fmt.Fprintf(&text, "0x%x  %-12s  %s\n", ins.address, formatInstruction(ins), mnemonic(ins))
	}
	return text.String()
}

// formatInstruction prints the encoded bytes of one instruction.
func formatInstruction(ins instruction) string {
	return fmt.Sprintf("%02x %02x %02x %02x",
		byte(ins.raw), byte(ins.raw>>8), byte(ins.raw>>16), byte(ins.raw>>24))
}

// mnemonic renders one instruction as text.
func mnemonic(ins instruction) string {
	if ins.undefined {
		return fmt.Sprintf(".word 0x%08x", ins.raw)
	}
	return strings.TrimSpace(ins.inst.String())
}

var _ = loong64asm.R0
