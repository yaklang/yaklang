package arm64

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func (Parser) Disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	if err := (Parser{}).Check(request.Target); err != nil {
		return nil, err
	}
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("arm64: empty code region")
	}
	diagnostics := &diagnostics{}
	decoded, err := decode(ctx, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		message := fmt.Sprintf("arm64: no instruction decoded at 0x%x", request.Address)
		if len(diagnostics.items) > 0 {
			message += ": " + diagnostics.items[0].Message
		}
		return nil, fmt.Errorf("%s", message)
	}
	end := request.Address + uint64(request.Reader.Size())
	functions, blockIDs := buildFunctions(decoded, request.Address, end, diagnostics)
	// Assembly and LowIR are two views of the same pipeline. Lift anyway so a
	// region cannot look clean only because the caller asked for text.
	_ = liftFunctions(request.Target, functions, blockIDs, diagnostics)
	return &core.Disassembly{
		Text:         renderAssembly(decoded),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

// renderAssembly prints one instruction per line. AArch64 has a single
// assembly syntax, so the requested dialect does not change the text.
func renderAssembly(decoded []instruction) string {
	var text strings.Builder
	for _, ins := range decoded {
		mnemonic := ins.text
		switch {
		case ins.undefined:
			mnemonic = fmt.Sprintf("udf #%#x", ins.immediate)
		case mnemonic == "":
			mnemonic = strings.TrimSpace(arm64asm.GNUSyntax(ins.inst))
		}
		fmt.Fprintf(&text, "0x%x  %-32s  %s\n", ins.address, formatInstruction(ins.raw), mnemonic)
	}
	return text.String()
}

func formatInstruction(raw uint32) string {
	return fmt.Sprintf("%02x %02x %02x %02x",
		byte(raw), byte(raw>>8), byte(raw>>16), byte(raw>>24))
}
