package pass

import "github.com/yaklang/yaklang/common/yak/ssa"

// SimplifyPartialWrites removes the `old & ~0xff | low & 0xff` merge that is
// produced by a byte register write followed by a zero-extension. The low-byte
// value is the actual result of that sequence.
func SimplifyPartialWrites(program *ssa.Program) int {
	if program == nil {
		return 0
	}
	replaced := 0
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				or, ok := instruction.(*ssa.BinOp)
				if !ok || or == nil || or.Op != ssa.OpOr {
					continue
				}
				left, right := orBinOpValue(or, or.X), orBinOpValue(or, or.Y)
				if left == nil || right == nil {
					continue
				}
				var low *ssa.BinOp
				if isMaskedOr(or, left, right) {
					low = right
				} else if isMaskedOr(or, right, left) {
					low = left
				}
				if low == nil || low.X == 0 {
					continue
				}
				source, ok := or.GetValueById(low.X)
				if !ok || source == nil {
					continue
				}
				ssa.ReplaceValue(or, source, func(instruction ssa.Instruction) bool {
					return instruction == or
				})
				replaced++
			}
		}
		return true
	})
	return replaced
}

func orBinOpValue(instruction ssa.Instruction, id int64) *ssa.BinOp {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return nil
	}
	bin, _ := value.(*ssa.BinOp)
	return bin
}

func isMaskedOr(instruction ssa.Instruction, high, low *ssa.BinOp) bool {
	return high != nil && low != nil &&
		high.Op == ssa.OpAndNot && low.Op == ssa.OpAnd &&
		isMask255(instruction, high.Y) && isMask255(instruction, low.Y)
}

func isMask255(instruction ssa.Instruction, id int64) bool {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return false
	}
	if cast, ok := ssa.ToTypeCast(value); ok && cast != nil {
		value, ok = cast.GetValueById(cast.Value)
		if !ok || value == nil {
			return false
		}
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil {
		return false
	}
	switch raw := constant.GetRawValue().(type) {
	case int:
		return raw == 255
	case int64:
		return raw == 255
	case uint64:
		return raw == 255
	case uint32:
		return raw == 255
	default:
		return false
	}
}
