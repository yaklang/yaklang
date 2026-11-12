package ppc

import (
	"context"
	"encoding/binary"
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
		Text:         renderAssembly(decoded, request.Target.ByteOrder),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

// renderAssembly prints one instruction per line. PowerPC has a single assembly
// syntax, so the requested dialect does not change the text.
func renderAssembly(decoded []instruction, order core.ByteOrder) string {
	var text strings.Builder
	var byteOrder binary.ByteOrder = binary.BigEndian
	if order == core.LittleEndian {
		byteOrder = binary.LittleEndian
	}
	for _, ins := range decoded {
		mnemonic := mnemonicText(ins)
		if ins.undefined {
			mnemonic = fmt.Sprintf(".long 0x%08x", ins.raw)
		}
		fmt.Fprintf(&text, "0x%x  %-16s  %s\n", ins.address, formatInstruction(ins.raw, byteOrder), mnemonic)
	}
	return text.String()
}

func formatInstruction(raw uint32, order binary.ByteOrder) string {
	var buffer [4]byte
	order.PutUint32(buffer[:], raw)
	return fmt.Sprintf("%02x %02x %02x %02x", buffer[0], buffer[1], buffer[2], buffer[3])
}

// mnemonicText renders one instruction. PowerPC prints the branch-register forms
// with their condition and option fields, which hides the common aliases; the
// aliases are restored so the text matches the assembler spelling.
func mnemonicText(ins instruction) string {
	text := ins.inst.String()
	name := normalize(opName(ins.inst.Op))
	switch name {
	case "bclr":
		// bclr 20, 0, 0 is blr; bclr 20, 1, 0 is blrl.
		if strings.HasPrefix(text, "bclr 20, Cond0") || strings.HasPrefix(text, "bclr 20,") {
			if strings.Contains(text, ", 0") && !strings.Contains(text, ", 1") {
				return "blr"
			}
			return "bclrl"
		}
	case "bcctr":
		return strings.Replace(text, "bcctr 20,", "bctr", 1)
	}
	return text
}
