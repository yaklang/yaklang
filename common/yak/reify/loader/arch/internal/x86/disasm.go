package x86

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func disassemble(ctx context.Context, profile Profile, request core.ParseRequest) (*core.Disassembly, error) {
	if err := profile.Check(request.Target); err != nil {
		return nil, err
	}
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("%s: empty code region", profile.Name)
	}
	diagnostics := &diagnostics{profile: profile}
	decoded, err := decodeInstructions(ctx, profile, request, diagnostics, true)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		message := fmt.Sprintf("%s: no instruction decoded at 0x%x", profile.Name, request.Address)
		if len(diagnostics.items) > 0 {
			message += ": " + diagnostics.items[0].Message
		}
		return nil, fmt.Errorf("%s", message)
	}
	functions, blockIDs := buildFunctions(decoded, diagnostics)
	// Assembly and LowIR are two views of the same decode-and-lift pipeline.
	// Report lift diagnostics in both modes so a region cannot look clean only
	// because the caller asked for text.
	_ = liftFunctions(profile, request.Target, functions, blockIDs, diagnostics)
	return &core.Disassembly{
		Text:         renderAssembly(profile, decoded, request.Syntax),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

func renderAssembly(profile Profile, decoded []instruction, syntax core.AsmSyntax) string {
	var text strings.Builder
	for _, instruction := range decoded {
		fmt.Fprintf(&text, "0x%x  %-24s  %s\n", instruction.address, formatInstructionBytes(instruction.raw), instruction.text(profile, syntax))
	}
	return text.String()
}

func (i instruction) text(profile Profile, syntax core.AsmSyntax) string {
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
