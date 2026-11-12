package riscv

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func disassemble(ctx context.Context, profile Profile, request core.ParseRequest) (*core.Disassembly, error) {
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("%s: empty code region", profile.Name)
	}
	diagnostics := &diagnostics{profile: profile}
	decoded, err := decodeInstructions(ctx, profile, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("%s: no instruction decoded at 0x%x", profile.Name, request.Address)
	}
	end := request.Address + uint64(request.Reader.Size())
	functions, blockIDs := buildFunctions(profile, decoded, request.Address, end, diagnostics)
	_ = liftFunctions(profile, request.Target, functions, blockIDs, diagnostics)
	return &core.Disassembly{
		Text:         renderAssembly(decoded),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

// renderAssembly prints one instruction per line. RISC-V has a single assembly
// syntax, so the requested dialect does not change the text.
func renderAssembly(decoded []instruction) string {
	var text strings.Builder
	for _, ins := range decoded {
		mnemonic := ins.inst.String()
		if ins.undefined {
			mnemonic = fmt.Sprintf(".word 0x%08x", ins.raw)
		}
		fmt.Fprintf(&text, "0x%x  %-16s  %s\n", ins.address, formatInstruction(ins.raw, ins.size), mnemonic)
	}
	return text.String()
}

func formatInstruction(raw uint32, size int) string {
	switch size {
	case 2:
		return fmt.Sprintf("%02x %02x", byte(raw), byte(raw>>8))
	default:
		return fmt.Sprintf("%02x %02x %02x %02x", byte(raw), byte(raw>>8), byte(raw>>16), byte(raw>>24))
	}
}
