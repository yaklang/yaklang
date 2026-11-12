package s390x

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/arch/s390x/s390xasm"

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
		fmt.Fprintf(&text, "0x%x  %-18s  %s\n", ins.address, formatInstruction(ins), mnemonic(ins))
	}
	return text.String()
}

// formatInstruction prints the encoded bytes of one instruction.
func formatInstruction(ins instruction) string {
	parts := make([]string, 0, ins.size)
	for index := 0; index < ins.size; index++ {
		parts = append(parts, fmt.Sprintf("%02x", ins.encoded[index]))
	}
	return strings.Join(parts, " ")
}

// mnemonic renders one instruction as text. The library's String method opens a
// parenthesis for every displacement it prints and closes it only when a base
// register follows. A shift-by-immediate reuses the memory format with no base,
// so the text needs repair rather than a rewrite.
func mnemonic(ins instruction) string {
	if ins.undefined {
		return fmt.Sprintf(".word 0x%02x%02x", ins.encoded[0], ins.encoded[1])
	}
	text := ins.inst.String(ins.address)
	if strings.Count(text, "(") <= strings.Count(text, ")") {
		return text
	}
	if hasIndexOperand(ins.inst) {
		// The operand is memory with an index register and no base. The index
		// already printed its trailing comma, so only the base is missing.
		return text + "0)"
	}
	// The operand is an immediate: the parenthesis does not belong.
	return strings.TrimSuffix(text, "(")
}

// hasIndexOperand reports whether the instruction carries an index register.
func hasIndexOperand(inst s390xasm.Inst) bool {
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		if index, ok := arg.(s390xasm.Index); ok {
			if _, named := indexRegisterName(index); named {
				return true
			}
		}
	}
	return false
}
