package pass

import "github.com/yaklang/yaklang/common/yak/reify/core"

// FoldConstants replaces an arithmetic operation whose operands are both
// constants with the constant result.
//
// A target reaches its data through a chain an assembler writes in pieces: a
// high half, a low half, and a base register that itself holds a known address.
// Every step is a machine instruction, so the chain reaches the converter as
// separate operations on constants. Folding it is what turns the chain into one
// address, and only then can the renderer name the string or global it points at.
//
// A shift whose count is at least the operand width is left alone: the result
// would be undefined and guessing is worse than keeping the expression.
func FoldConstants(function *core.Function) int {
	if function == nil || len(function.Blocks) == 0 {
		return 0
	}
	constants := make(map[core.ValueID]uint64)
	for pass := 0; pass < 8; pass++ {
		folded := 0
		for blockIndex := range function.Blocks {
			block := &function.Blocks[blockIndex]
			for index := range block.Instructions {
				instruction := &block.Instructions[index]
				if len(instruction.Results) != 1 || len(instruction.Operands) == 0 {
					continue
				}
				// A copy of a constant is the constant. The lifters use a copy to
				// give an address a pointer type, and a chain mixes both forms.
				if instruction.Kind == core.OpCopy {
					if value, ok := constants[instruction.Operands[0].Value]; ok {
						foldToConstant(instruction, value)
						constants[instruction.Results[0].ID] = value
						folded++
					}
					continue
				}
				if len(instruction.Operands) != 2 {
					continue
				}
				if instruction.Kind == core.OpConstant {
					if value, ok := constantLiteral(instruction.Operands[0]); ok {
						constants[instruction.Results[0].ID] = value
					}
					continue
				}
				left, ok := constants[instruction.Operands[0].Value]
				if !ok {
					continue
				}
				right, ok := constants[instruction.Operands[1].Value]
				if !ok {
					continue
				}
				value, ok := foldKind(instruction.Kind, left, right, instruction.Results[0].Type.Bits)
				if !ok {
					continue
				}
				foldToConstant(instruction, value)
				constants[instruction.Results[0].ID] = value
				folded++
			}
		}
		if folded == 0 {
			return 0
		}
	}
	return 0
}

// foldToConstant rewrites one instruction as a constant with the same result.
func foldToConstant(instruction *core.Instruction, value uint64) {
	result := instruction.Results[0]
	*instruction = core.Instruction{
		Kind:     core.OpConstant,
		Operands: []core.Operand{{Kind: core.OperandConstant, Type: result.Type, Literal: constantBytes(value, result.Type.Bits)}},
		Results:  []core.Value{result},
		Effects:  core.EffectNone,
		Address:  instruction.Address,
	}
}

// foldKind computes one operation on two constants.
func foldKind(kind core.Opcode, left, right uint64, bits uint16) (uint64, bool) {
	width := bits
	if width == 0 || width > 64 {
		width = 64
	}
	switch kind {
	case core.OpAdd:
		return left + right, true
	case core.OpSub:
		return left - right, true
	case core.OpMul:
		return left * right, true
	case core.OpShl:
		if right >= uint64(width) {
			return 0, false
		}
		return left << right, true
	case core.OpLShr:
		if right >= uint64(width) {
			return 0, false
		}
		return left >> right, true
	case core.OpAShr:
		if right >= uint64(width) {
			return 0, false
		}
		shifted := int64(left) >> right
		return uint64(shifted), true
	case core.OpAnd:
		return left & right, true
	case core.OpOr:
		return left | right, true
	case core.OpXor:
		return left ^ right, true
	}
	return 0, false
}

// constantLiteral reads the value of one constant operand.
func constantLiteral(operand core.Operand) (uint64, bool) {
	if operand.Kind != core.OperandConstant {
		return 0, false
	}
	var value uint64
	for index := len(operand.Literal) - 1; index >= 0; index-- {
		value = value<<8 | uint64(operand.Literal[index])
	}
	return value, true
}

// literal renders one constant at the given width.
func constantBytes(value uint64, bits uint16) []byte {
	size := int(bits / 8)
	if size == 0 || size > 8 {
		size = 8
	}
	result := make([]byte, size)
	for index := range result {
		result[index] = byte(value >> (8 * index))
	}
	return result
}
