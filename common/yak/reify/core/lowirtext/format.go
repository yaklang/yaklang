// Package lowirtext renders and parses the stable LowIR text form.
package lowirtext

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
