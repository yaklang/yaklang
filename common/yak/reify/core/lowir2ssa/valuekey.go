package lowir2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// valueKey builds a stable structural key for a value. Memory loads and stores
// use it to share one variable when they address the same constant, register or
// address expression.
func valueKey(defs map[core.ValueID]core.Instruction, id core.ValueID) string {
	return valueKeyDepth(defs, id, 0, make(map[core.ValueID]bool))
}

func valueKeyDepth(defs map[core.ValueID]core.Instruction, id core.ValueID, depth int, visiting map[core.ValueID]bool) string {
	if depth > 16 || visiting[id] {
		return fmt.Sprintf("v%d", id)
	}
	instruction, ok := defs[id]
	if !ok {
		return fmt.Sprintf("v%d", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	switch instruction.Kind {
	case core.OpConstant:
		if len(instruction.Operands) == 0 {
			return ""
		}
		return fmt.Sprintf("const:%x", literalUint64(instruction.Operands[0].Literal))
	case core.OpCopy:
		if len(instruction.Operands) == 0 {
			return ""
		}
		return valueKeyDepth(defs, instruction.Operands[0].Value, depth+1, visiting)
	case core.OpAdd:
		if len(instruction.Operands) < 2 {
			return ""
		}
		return fmt.Sprintf("add(%s,%s)",
			valueKeyDepth(defs, instruction.Operands[0].Value, depth+1, visiting),
			valueKeyDepth(defs, instruction.Operands[1].Value, depth+1, visiting))
	case core.OpReadRegister:
		if len(instruction.Operands) == 0 {
			return ""
		}
		return "reg:" + instruction.Operands[0].Name
	case core.OpIntrinsic:
		if len(instruction.Operands) == 0 {
			return ""
		}
		key := "intrinsic:" + instruction.Operands[0].Name
		for _, operand := range instruction.Operands[1:] {
			if operand.Kind == core.OperandValue {
				key += "(" + valueKeyDepth(defs, operand.Value, depth+1, visiting) + ")"
			}
		}
		return key
	default:
		return fmt.Sprintf("v%d", id)
	}
}
