package llvmxor

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

type xorLLVMObfuscator struct{}

func init() {
	core.Register(xorLLVMObfuscator{})
}

func (xorLLVMObfuscator) Name() string {
	return "xor"
}

func (xorLLVMObfuscator) Kind() core.Kind {
	return core.KindLLVM
}

// Run rewrites integer LLVM add/sub instructions into xor-based arithmetic forms.
//
// The local LLVM rewrites are:
//
//	add(x, y) => add(xor(x, y), shl(and(x, y), 1))
//	sub(x, y) => sub(xor(x, y), shl(and(xor(x, -1), y), 1))
//
// This keeps the arithmetic equivalent while changing the IR shape into a small
// xor/and/shift network that can be matched directly in tests.
func (xorLLVMObfuscator) Apply(ctx *core.Context) error {
	if ctx == nil || ctx.Stage != core.StageLLVM {
		return nil
	}
	module := ctx.LLVM
	if module.C == nil {
		return nil
	}

	builder := module.NewBuilder()
	defer builder.Dispose()

	rewriteIndex := uint64(0)
	for function := module.FirstFunction(); !function.IsNil(); function = function.NextFunction() {
		for block := function.FirstBasicBlock(); !block.IsNil(); block = block.NextBasicBlock() {
			for inst := block.FirstInstruction(); !inst.IsNil(); {
				next := inst.NextInstruction()
				if shouldRewrite(inst) {
					if err := rewriteArithmetic(builder, inst, rewriteIndex); err != nil {
						return err
					}
					rewriteIndex++
				}
				inst = next
			}
		}
	}

	return nil
}

func shouldRewrite(inst llvm.Value) bool {
	if inst.IsNil() {
		return false
	}

	switch inst.InstructionOpcode() {
	case llvm.Add, llvm.Sub:
	default:
		return false
	}

	instType := inst.Type()
	if !instType.IsInteger() {
		return false
	}
	return instType.IntTypeWidth() > 1
}

func rewriteArithmetic(builder llvm.Builder, inst llvm.Value, rewriteIndex uint64) error {
	if inst.NumOperands() < 2 {
		return fmt.Errorf("llvm xor obfuscator expects two operands")
	}

	left := inst.Operand(0)
	right := inst.Operand(1)
	one := llvm.ConstInt(inst.Type(), 1, false)
	allOnes := llvm.ConstAllOnes(inst.Type())
	prefix := fmt.Sprintf("obf_xor_%d", rewriteIndex)

	builder.SetInsertPointBefore(inst)

	xorValue := builder.CreateXor(left, right, prefix+"_xor")

	var replacement llvm.Value
	switch inst.InstructionOpcode() {
	case llvm.Add:
		// LLVM IR transform:
		//   %orig = add iN %left, %right
		// becomes
		//   %xor = xor %left, %right
		//   %and = and %left, %right
		//   %shl = shl %and, 1
		//   %res = add %xor, %shl
		carryMask := builder.CreateAnd(left, right, prefix+"_and")
		carryShift := builder.CreateShl(carryMask, one, prefix+"_shl")
		replacement = builder.CreateAdd(xorValue, carryShift, prefix+"_res")
	case llvm.Sub:
		// LLVM IR transform:
		//   %orig = sub iN %left, %right
		// becomes
		//   %xor = xor %left, %right
		//   %not = xor %left, -1
		//   %and = and %not, %right
		//   %shl = shl %and, 1
		//   %res = sub %xor, %shl
		notLeft := builder.CreateXor(left, allOnes, prefix+"_not")
		borrowMask := builder.CreateAnd(notLeft, right, prefix+"_and")
		borrowShift := builder.CreateShl(borrowMask, one, prefix+"_shl")
		replacement = builder.CreateSub(xorValue, borrowShift, prefix+"_res")
	default:
		return fmt.Errorf("unsupported llvm opcode %d", int(inst.InstructionOpcode()))
	}

	inst.ReplaceAllUsesWith(replacement)
	inst.EraseFromParent()
	return nil
}
