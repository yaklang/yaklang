package amd64

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func (Parser) Disassemble(ctx context.Context, request core.ParseRequest) (*core.Disassembly, error) {
	if err := (Parser{}).Check(request.Target); err != nil {
		return nil, err
	}
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("amd64: empty code region")
	}
	diagnostics := &amd64Diagnostics{}
	decoded, err := decodeAMD64(ctx, request, diagnostics, true)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		message := fmt.Sprintf("amd64: no instruction decoded at 0x%x", request.Address)
		if len(diagnostics.items) > 0 {
			message += ": " + diagnostics.items[0].Message
		}
		return nil, fmt.Errorf("%s", message)
	}
	blocks, blockIDs := buildAMD64Blocks(decoded, diagnostics)
	// Assembly and LowIR are two views of the same decode-and-lift pipeline.
	// Report lift diagnostics in both modes so a region cannot look clean only
	// because the caller asked for text.
	_ = liftAMD64(request.Target, blocks, blockIDs, diagnostics)
	return &core.Disassembly{
		Text:         renderAMD64Assembly(decoded, request.Syntax),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

func renderAMD64Assembly(decoded []amd64Instruction, syntax core.AsmSyntax) string {
	var text strings.Builder
	for _, instruction := range decoded {
		fmt.Fprintf(&text, "0x%x  %-24s  %s\n", instruction.address, formatInstructionBytes(instruction.raw), instruction.text(syntax))
	}
	return text.String()
}

func (i amd64Instruction) text(syntax core.AsmSyntax) string {
	if i.endbr != "" {
		return i.endbr
	}
	// Multi-byte NOP padding encodes its length in the ModRM/SIB bytes. The CPU
	// does not read that memory, so printing "nopw %cs:(...)" is misleading.
	if i.inst.Op == x86asm.NOP {
		return "nop"
	}
	if syntax == core.AsmSyntaxATT {
		return x86asm.GNUSyntax(i.inst, i.address, nil)
	}
	return x86asm.IntelSyntax(i.inst, i.address, nil)
}

func formatInstructionBytes(raw []byte) string {
	var text strings.Builder
	for index, value := range raw {
		if index > 0 {
			text.WriteByte(' ')
		}
		fmt.Fprintf(&text, "%02x", value)
	}
	return text.String()
}
