package core

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// FormatLowIR renders LowIR as a stable, line-oriented text form. The output is
// for inspection; JSON remains the machine-readable form.
func FormatLowIR(ir *LowIR) string {
	if ir == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "target: %s/%s (%d-bit, %s-endian)\n", ir.Target.OS, ir.Target.Architecture, ir.Target.Bits, ir.Target.ByteOrder)
	for _, function := range ir.Functions {
		fmt.Fprintf(&b, "function f%d @ 0x%x\n", function.ID, function.Address)
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
				if instruction.Effects != EffectNone {
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

func formatValueType(t ValueType) string {
	switch t.Kind {
	case TypeInteger:
		return fmt.Sprintf("i%d", t.Bits)
	case TypePointer:
		return fmt.Sprintf("ptr%d", t.Bits)
	case TypeFloat:
		return fmt.Sprintf("f%d", t.Bits)
	default:
		if t.Bits == 0 {
			return "opaque"
		}
		return fmt.Sprintf("opaque%d", t.Bits)
	}
}

func formatOperand(operand Operand) string {
	switch operand.Kind {
	case OperandValue:
		return fmt.Sprintf("v%d", operand.Value)
	case OperandConstant:
		return formatLiteral(operand.Literal)
	case OperandRegister:
		return "%" + operand.Name
	case OperandSymbol:
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

func formatEffects(effects Effect) string {
	names := []struct {
		flag Effect
		name string
	}{
		{EffectReadMemory, "read-memory"},
		{EffectWriteMemory, "write-memory"},
		{EffectReadRegister, "read-register"},
		{EffectWriteRegister, "write-register"},
		{EffectCall, "call"},
		{EffectMayTrap, "may-trap"},
		{EffectVolatile, "volatile"},
		{EffectAtomic, "atomic"},
		{EffectUnknown, "unknown"},
	}
	var parts []string
	var known Effect
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
