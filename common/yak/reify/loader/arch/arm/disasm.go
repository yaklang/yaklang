package arm

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func (Parser) Disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	if err := (Parser{}).Check(request.Target); err != nil {
		return nil, err
	}
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("arm: empty code region")
	}
	diagnostics := &diagnostics{}
	decoded, err := decode(ctx, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("arm: no instruction decoded at 0x%x", request.Address)
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

// renderAssembly prints one instruction per line. ARM has a single assembly
// syntax, so the requested dialect does not change the text.
func renderAssembly(decoded []instruction) string {
	var text strings.Builder
	for _, ins := range decoded {
		mnemonic := ins.text
		switch {
		case ins.undefined:
			mnemonic = fmt.Sprintf("udf #%#x", ins.raw&0xffff)
		case mnemonic == "":
			mnemonic = strings.TrimSpace(armasm.GNUSyntax(ins.inst))
		}
		fmt.Fprintf(&text, "0x%x  %-20s  %s\n", ins.address, formatInstruction(ins.raw, ins.size), mnemonic)
	}
	return text.String()
}

// formatInstruction prints the encoded bytes. A Thumb instruction is either one
// halfword or two, so the raw word holds one or two of them.
func formatInstruction(raw uint32, size int) string {
	if size == 2 {
		return fmt.Sprintf("%02x %02x", byte(raw), byte(raw>>8))
	}
	return fmt.Sprintf("%02x %02x %02x %02x",
		byte(raw), byte(raw>>8), byte(raw>>16), byte(raw>>24))
}
