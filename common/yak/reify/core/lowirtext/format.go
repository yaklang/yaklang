// Package lowirtext renders and parses the stable LowIR text form.
package lowirtext

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// Format renders LowIR as a stable, line-oriented text form.
func Format(ir *core.LowIR) string {
	if ir == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "target: %s/%s (%d-bit, %s-endian)\n", ir.Target.OS, ir.Target.Architecture, ir.Target.Bits, ir.Target.ByteOrder)
	// The architecture facts are data, so the text form has to carry them:
	// without them a pass cannot name a stack pointer or bind an argument
	// register, and a round trip would silently change what the program means.
	if line := formatFrame(ir.Frame); line != "" {
		b.WriteString(line)
	}
	if line := formatRegisters(ir.Registers); line != "" {
		b.WriteString(line)
	}
	if line := formatCanary(ir.Canary); line != "" {
		b.WriteString(line)
	}
	for _, function := range ir.Functions {
		if function.Name != "" {
			fmt.Fprintf(&b, "function %s @ 0x%x\n", function.Name, function.Address)
		} else {
			fmt.Fprintf(&b, "function f%d @ 0x%x\n", function.ID, function.Address)
		}
		for _, block := range function.Blocks {
			fmt.Fprintf(&b, "block b%d @ 0x%x\n", block.ID, block.Address)
			for _, instruction := range block.Instructions {
				fmt.Fprintf(&b, "  0x%x  ", instruction.Address)
				if len(instruction.Results) > 0 {
					results := make([]string, 0, len(instruction.Results))
					for _, result := range instruction.Results {
						results = append(results, fmt.Sprintf("v%d:%s", result.ID, formatValueType(result.Type)))
					}
					b.WriteString(strings.Join(results, ", "))
					b.WriteString(" = ")
				}
				b.WriteString(string(instruction.Kind))
				if len(instruction.Operands) > 0 {
					operands := make([]string, 0, len(instruction.Operands))
					for _, operand := range instruction.Operands {
						operands = append(operands, formatOperand(operand))
					}
					b.WriteString(" ")
					b.WriteString(strings.Join(operands, ", "))
				}
				if len(instruction.Targets) > 0 {
					targets := make([]string, 0, len(instruction.Targets))
					for _, target := range instruction.Targets {
						targets = append(targets, fmt.Sprintf("b%d", target))
					}
					b.WriteString(" -> ")
					b.WriteString(strings.Join(targets, ", "))
				}
				if instruction.Effects != core.EffectNone {
					b.WriteString(" [")
					b.WriteString(formatEffects(instruction.Effects))
					b.WriteString("]")
				}
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func formatValueType(t core.ValueType) string {
	switch t.Kind {
	case core.TypeInteger:
		return fmt.Sprintf("i%d", t.Bits)
	case core.TypePointer:
		return fmt.Sprintf("ptr%d", t.Bits)
	case core.TypeFloat:
		return fmt.Sprintf("f%d", t.Bits)
	default:
		if t.Bits == 0 {
			return "opaque"
		}
		return fmt.Sprintf("opaque%d", t.Bits)
	}
}

func formatOperand(operand core.Operand) string {
	switch operand.Kind {
	case core.OperandValue:
		return fmt.Sprintf("v%d", operand.Value)
	case core.OperandConstant:
		return formatLiteral(operand.Literal)
	case core.OperandRegister:
		return "%" + operand.Name
	case core.OperandSymbol:
		return "@" + operand.Name
	default:
		return string(operand.Kind)
	}
}

func formatLiteral(literal []byte) string {
	if len(literal) == 0 {
		return "0x0"
	}
	if len(literal) <= 8 {
		var padded [8]byte
		copy(padded[:], literal)
		return fmt.Sprintf("0x%x", binary.LittleEndian.Uint64(padded[:]))
	}
	return "0x" + hex.EncodeToString(literal)
}

func formatEffects(effects core.Effect) string {
	names := []struct {
		flag core.Effect
		name string
	}{
		{core.EffectReadMemory, "read_memory"},
		{core.EffectWriteMemory, "write_memory"},
		{core.EffectReadRegister, "read_register"},
		{core.EffectWriteRegister, "write_register"},
		{core.EffectCall, "call"},
		{core.EffectMayTrap, "may_trap"},
		{core.EffectVolatile, "volatile"},
		{core.EffectAtomic, "atomic"},
		{core.EffectUnknown, "unknown"},
		{core.EffectFrameRestore, "frame_restore"},
	}
	var parts []string
	var known core.Effect
	for _, name := range names {
		known |= name.flag
		if effects&name.flag != 0 {
			parts = append(parts, name.name)
		}
	}
	if remainder := effects &^ known; remainder != 0 {
		parts = append(parts, fmt.Sprintf("0x%x", uint32(remainder)))
	}
	return strings.Join(parts, ", ")
}

// formatFrame renders the frame model as one line of key=value pairs. An absent
// field is omitted so the line stays readable.
func formatFrame(frame core.FrameModel) string {
	parts := make([]string, 0, 6)
	if frame.StackPointer != "" {
		parts = append(parts, "sp="+frame.StackPointer)
	}
	if frame.FramePointer != "" {
		parts = append(parts, "fp="+frame.FramePointer)
	}
	if frame.ReturnAddress != "" {
		parts = append(parts, "ret="+frame.ReturnAddress)
	}
	if frame.Flags != "" {
		parts = append(parts, "flags="+frame.Flags)
	}
	if len(frame.CalleeSaved) > 0 {
		parts = append(parts, "saved="+strings.Join(frame.CalleeSaved, ","))
	}
	if frame.StackGrowth != 0 {
		parts = append(parts, "growth="+strconv.Itoa(int(frame.StackGrowth)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "frame: " + strings.Join(parts, " ") + "\n"
}

// formatRegisters renders the register model as one line of key=value pairs.
func formatRegisters(registers core.RegisterModel) string {
	parts := make([]string, 0, 6)
	if registers.Naming != "" {
		parts = append(parts, "naming="+registers.Naming)
	}
	if registers.Width != 0 {
		parts = append(parts, "width="+strconv.Itoa(int(registers.Width)))
	}
	if registers.Flags != "" {
		parts = append(parts, "flags="+registers.Flags)
	}
	if registers.FlagsWidth != 0 {
		parts = append(parts, "flags_width="+strconv.Itoa(int(registers.FlagsWidth)))
	}
	if len(registers.Arguments) > 0 {
		parts = append(parts, "args="+strings.Join(registers.Arguments, ","))
	}
	if registers.Return != "" {
		parts = append(parts, "return="+registers.Return)
	}
	if len(parts) == 0 {
		return ""
	}
	return "registers: " + strings.Join(parts, " ") + "\n"
}

// formatCanary renders the stack guard model as one line of key=value pairs.
func formatCanary(canary core.CanaryModel) string {
	parts := make([]string, 0, 4)
	if canary.Kind != "" {
		parts = append(parts, "kind="+canary.Kind)
	}
	if canary.Register != "" {
		parts = append(parts, "register="+canary.Register)
	}
	if canary.Offset != 0 {
		parts = append(parts, fmt.Sprintf("offset=0x%x", canary.Offset))
	}
	if canary.Symbol != "" {
		parts = append(parts, "symbol="+canary.Symbol)
	}
	if len(parts) == 0 {
		return ""
	}
	return "canary: " + strings.Join(parts, " ") + "\n"
}
