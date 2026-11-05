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
	var text strings.Builder
	for _, instruction := range decoded {
		fmt.Fprintf(&text, "0x%x  %-24s  %s\n", instruction.address, formatInstructionBytes(instruction.raw), instruction.text())
	}
	return &core.Disassembly{
		Text:         text.String(),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

func (i amd64Instruction) text() string {
	if i.endbr != "" {
		return i.endbr
	}
	return x86asm.GNUSyntax(i.inst, i.address, nil)
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
