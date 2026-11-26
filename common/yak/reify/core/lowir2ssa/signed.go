package lowir2ssa

import (
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// integer signedness is a property of the operation in LowIR, not of a value's
// bit pattern. The converter carries it into YakSSA by typing the operands and
// the result of a signed operation as intN and of an unsigned operation as iN,
// and by normalizing constants to the operation's signedness before it folds
// them. Without that step a 32-bit 0xffffffff compares as a positive number,
// and a division or shift follows the unsigned C rules of the printed type.

// widthOfSSAType returns the bit width encoded in a type name such as i32,
// int64 or ptr64.
func widthOfSSAType(typ ssa.Type) uint16 {
	if typ == nil {
		return 0
	}
	name := typ.String()
	start := len(name)
	for start > 0 && name[start-1] >= '0' && name[start-1] <= '9' {
		start--
	}
	if start == len(name) {
		return 0
	}
	bits, err := strconv.ParseUint(name[start:], 10, 16)
	if err != nil {
		return 0
	}
	return uint16(bits)
}

func isSignedIntegerType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	return len(name) > 3 && name[:3] == "int"
}

// signedTypeFor returns the signed integer type of one width.
func (c *converter) signedTypeFor(bits uint16) ssa.Type {
	if bits == 0 {
		bits = 64
	}
	if typ, ok := c.signedTypes[bits]; ok {
		return typ
	}
	typ := ssa.NewBasicType(ssa.NumberTypeKind, fmt.Sprintf("int%d", bits))
	c.signedTypes[bits] = typ
	return typ
}

// operandWidth returns the width a value is declared with. The operand can name
// a value whose width the LowIR result carries, so the value table is consulted
// before the operand's own type.
func (c *converter) operandWidth(operand core.Operand, fallback uint16) uint16 {
	if operand.Kind == core.OperandValue {
		if value, ok := c.values[operand.Value]; ok && value != nil {
			if bits := widthOfSSAType(value.GetType()); bits > 0 {
				return bits
			}
		}
		if definition, ok := c.defs[operand.Value]; ok && len(definition.Results) > 0 {
			if bits := definition.Results[0].Type.Bits; bits > 0 {
				return bits
			}
		}
	}
	if operand.Type.Kind == core.TypeInteger && operand.Type.Bits > 0 {
		return operand.Type.Bits
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}

// instructionWidth returns the width of the operation's left operand, falling
// back to the declared result width and the machine width.
func (c *converter) instructionWidth(instruction core.Instruction) uint16 {
	if len(instruction.Operands) > 0 {
		if bits := c.operandWidth(instruction.Operands[0], 0); bits > 0 {
			return bits
		}
	}
	if len(instruction.Results) > 0 && instruction.Results[0].Type.Bits > 0 {
		return instruction.Results[0].Type.Bits
	}
	if c.facts.Registers.Width > 0 {
		return c.facts.Registers.Width
	}
	return 64
}

// signedConstant truncates a constant to the declared width and sign-extends it,
// so a 32-bit 0xffffffff becomes the number -1 the machine sees.
func signedConstant(constant *ssa.ConstInst, bits uint16) (int64, bool) {
	number, ok := constInteger(constant)
	if !ok {
		return 0, false
	}
	if bits == 0 || bits >= 64 {
		return number, true
	}
	mask := int64(1)<<bits - 1
	sign := int64(1) << (bits - 1)
	number &= mask
	if number&sign != 0 {
		number -= int64(1) << bits
	}
	return number, true
}

func unsignedConstant(constant *ssa.ConstInst, bits uint16) (uint64, bool) {
	number, ok := constInteger(constant)
	if !ok {
		return 0, false
	}
	value := uint64(number)
	if bits > 0 && bits < 64 {
		value &= (uint64(1) << bits) - 1
	}
	return value, true
}

// normalizeSigned returns an operand whose SSA value is the signed number of the
// declared width. A constant is folded; any other value is cast to intN when its
// declared type does not already carry the width.
func (c *converter) normalizeSigned(value ssa.Value, bits uint16) ssa.Value {
	if value == nil {
		return nil
	}
	if constant, ok := ssa.ToConstInst(value); ok {
		if number, ok := signedConstant(constant, bits); ok {
			folded := c.builder.EmitConstInst(number)
			if folded != nil {
				folded.SetType(c.signedTypeFor(bits))
			}
			return folded
		}
	}
	if isSignedIntegerType(value.GetType()) && (bits == 0 || widthOfSSAType(value.GetType()) == bits) {
		return value
	}
	return c.castType(value, c.signedTypeFor(bits))
}

// normalizeUnsigned returns an operand interpreted without a sign. A constant is
// masked to the declared width; any other value is cast to iN, the unsigned
// spelling of the SSA integer.
func (c *converter) normalizeUnsigned(value ssa.Value, bits uint16) ssa.Value {
	if value == nil {
		return nil
	}
	if constant, ok := ssa.ToConstInst(value); ok {
		if number, ok := unsignedConstant(constant, bits); ok {
			folded := c.builder.EmitConstInst(number)
			if folded != nil {
				folded.SetType(c.typeFor(integerType(bits)))
			}
			return folded
		}
	}
	if !isSignedIntegerType(value.GetType()) && (bits == 0 || widthOfSSAType(value.GetType()) == bits) {
		return value
	}
	return c.castType(value, c.typeFor(integerType(bits)))
}

// orderedBinaryFolds computes a division or a shift with the signedness the
// opcode declares.
func orderedBinaryFolds(op ssa.BinaryOpcode, signed bool, left, right *ssa.ConstInst, bits uint16) (int64, bool) {
	if left == nil || right == nil {
		return 0, false
	}
	if signed {
		a, ok := signedConstant(left, bits)
		if !ok {
			return 0, false
		}
		b, ok := signedConstant(right, bits)
		if !ok {
			return 0, false
		}
		switch op {
		case ssa.OpDiv:
			if b == 0 {
				return 0, true
			}
			return a / b, true
		case ssa.OpShr:
			if b < 0 || b >= 64 {
				return 0, false
			}
			return a >> uint(b), true
		}
		return 0, false
	}
	a, ok := unsignedConstant(left, bits)
	if !ok {
		return 0, false
	}
	b, ok := unsignedConstant(right, bits)
	if !ok {
		return 0, false
	}
	switch op {
	case ssa.OpDiv:
		if b == 0 {
			return 0, true
		}
		return int64(a / b), true
	case ssa.OpShr:
		if b >= 64 {
			return 0, false
		}
		return int64(a >> uint(b)), true
	}
	return 0, false
}

// it, matching what the machine leaves in the register.
func truncateSigned(value int64, bits uint16) int64 {
	if bits == 0 || bits >= 64 {
		return value
	}
	mask := int64(1)<<bits - 1
	sign := int64(1) << (bits - 1)
	value &= mask
	if value&sign != 0 {
		value -= int64(1) << bits
	}
	return value
}

// emitOrderedBinary converts udiv/sdiv and lshr/ashr. The operands are
// normalized to the operation's signedness first, so both the constant folding
// and the rendered C expression follow the machine instruction.
func (c *converter) emitOrderedBinary(instruction core.Instruction, op ssa.BinaryOpcode, signed bool) {
	values := c.operandValues(instruction.Operands)
	if len(values) < 2 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("operation %q at 0x%x has %d operands", instruction.Kind, instruction.Address, len(values)))
		c.setResult(instruction, c.opaqueValue(string(instruction.Kind), instruction))
		return
	}
	bits := c.instructionWidth(instruction)
	left, right := values[0], values[1]
	if signed {
		left, right = c.normalizeSigned(left, bits), c.normalizeSigned(right, bits)
	} else {
		left, right = c.normalizeUnsigned(left, bits), c.normalizeUnsigned(right, bits)
	}
	resultType := c.typeFor(integerType(bits))
	if len(instruction.Results) > 0 {
		if signed {
			resultType = c.signedTypeFor(instruction.Results[0].Type.Bits)
		} else {
			resultType = c.typeFor(instruction.Results[0].Type)
		}
	}
	leftConstant, leftOK := ssa.ToConstInst(left)
	rightConstant, rightOK := ssa.ToConstInst(right)
	if leftOK && rightOK {
		if folded, ok := orderedBinaryFolds(op, signed, leftConstant, rightConstant, bits); ok {
			folded = truncateSigned(folded, bits)
			value := c.builder.EmitConstInst(folded)
			if value != nil {
				c.setResult(instruction, value)
				value.SetType(resultType)
			}
			return
		}
	}
	value := c.builder.EmitBinOp(op, left, right)
	if value == nil {
		return
	}
	c.setResult(instruction, value)
	value.SetType(resultType)
}

// orderedCompareFolds compares two constants with the signedness the opcode
// declares.
func orderedCompareFolds(op ssa.BinaryOpcode, signed bool, left, right *ssa.ConstInst, bits uint16) (bool, bool) {
	if signed {
		a, ok := signedConstant(left, bits)
		if !ok {
			return false, false
		}
		b, ok := signedConstant(right, bits)
		if !ok {
			return false, false
		}
		switch op {
		case ssa.OpEq:
			return a == b, true
		case ssa.OpNotEq:
			return a != b, true
		case ssa.OpLt:
			return a < b, true
		case ssa.OpLtEq:
			return a <= b, true
		case ssa.OpGt:
			return a > b, true
		case ssa.OpGtEq:
			return a >= b, true
		}
		return false, false
	}
	a, ok := unsignedConstant(left, bits)
	if !ok {
		return false, false
	}
	b, ok := unsignedConstant(right, bits)
	if !ok {
		return false, false
	}
	switch op {
	case ssa.OpEq:
		return a == b, true
	case ssa.OpNotEq:
		return a != b, true
	case ssa.OpLt:
		return a < b, true
	case ssa.OpLtEq:
		return a <= b, true
	case ssa.OpGt:
		return a > b, true
	case ssa.OpGtEq:
		return a >= b, true
	}
	return false, false
}

// emitOrderedCompare converts a comparison that has a signed and an unsigned
// form. Equality ignores the sign, so only the ordering opcodes need it.
func (c *converter) emitOrderedCompare(instruction core.Instruction, op ssa.BinaryOpcode, signed bool) ssa.Value {
	values := c.operandValues(instruction.Operands)
	if len(values) < 2 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("operation %q at 0x%x has %d operands", instruction.Kind, instruction.Address, len(values)))
		value := c.opaqueValue(string(instruction.Kind), instruction)
		c.setResult(instruction, value)
		return value
	}
	bits := c.instructionWidth(instruction)
	left, right := values[0], values[1]
	if signed {
		left, right = c.normalizeSigned(left, bits), c.normalizeSigned(right, bits)
	} else {
		left, right = c.normalizeUnsigned(left, bits), c.normalizeUnsigned(right, bits)
	}
	if leftConstant, ok := ssa.ToConstInst(left); ok {
		if rightConstant, ok := ssa.ToConstInst(right); ok {
			if result, ok := orderedCompareFolds(op, signed, leftConstant, rightConstant, bits); ok {
				value := c.builder.EmitConstInst(result)
				if value != nil {
					c.setResult(instruction, value)
				}
				return value
			}
		}
	}
	var value ssa.Value
	if left != nil && right != nil {
		value = c.builder.EmitBinOp(op, left, right)
	}
	if value == nil {
		value = c.opaqueValue(string(instruction.Kind), instruction)
	}
	c.setResult(instruction, value)
	return value
}

// orderedCompareSource emits a comparison between two already-converted values
// with the signedness the condition code declares.
func (c *converter) orderedCompareSource(op ssa.BinaryOpcode, left, right ssa.Value, signed bool) ssa.Value {
	if left == nil || right == nil {
		return nil
	}
	bits := widthOfSSAType(left.GetType())
	if bits == 0 {
		bits = widthOfSSAType(right.GetType())
	}
	if bits == 0 {
		bits = c.facts.Registers.Width
	}
	if signed {
		left, right = c.normalizeSigned(left, bits), c.normalizeSigned(right, bits)
	} else {
		left, right = c.normalizeUnsigned(left, bits), c.normalizeUnsigned(right, bits)
	}
	if leftConstant, ok := ssa.ToConstInst(left); ok {
		if rightConstant, ok := ssa.ToConstInst(right); ok {
			if result, ok := orderedCompareFolds(op, signed, leftConstant, rightConstant, bits); ok {
				return c.builder.EmitConstInst(result)
			}
		}
	}
	if left == nil || right == nil {
		return nil
	}
	return c.builder.EmitBinOp(op, left, right)
}
