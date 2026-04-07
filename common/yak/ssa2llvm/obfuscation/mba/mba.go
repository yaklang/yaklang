package mba

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

// mbaObfuscator rewrites integer arithmetic using Mixed Boolean-Arithmetic
// (MBA) identities. These identities mix bitwise and arithmetic operations
// to create expressions that are semantically equivalent but harder to
// simplify with standard compiler optimizations or pattern matching.
type mbaObfuscator struct{}

func init() {
	core.Register(mbaObfuscator{})
}

func (mbaObfuscator) Name() string { return "mba" }

func (mbaObfuscator) Kind() core.Kind { return core.KindLLVM }

func (mbaObfuscator) Apply(ctx *core.Context) error {
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
				if shouldRewriteMBA(inst) {
					if err := rewriteMBA(builder, inst, rewriteIndex); err != nil {
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

func shouldRewriteMBA(inst llvm.Value) bool {
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

// rewriteMBA applies MBA identities:
//
// add(x, y) => (x & y) + (x | y)
//   because: (x & y) + (x | y) = (x & y) + (x ^ y) + (x & y) = 2*(x & y) + (x ^ y) = x + y
//
// sub(x, y) => (x & ~y) - (~x & y)
//   because: (x & ~y) is bits only in x; (~x & y) is bits only in y;
//   difference gives x - y.
func rewriteMBA(builder llvm.Builder, inst llvm.Value, rewriteIndex uint64) error {
	if inst.NumOperands() < 2 {
		return fmt.Errorf("mba obfuscator expects two operands")
	}

	left := inst.Operand(0)
	right := inst.Operand(1)
	allOnes := llvm.ConstAllOnes(inst.Type())
	prefix := fmt.Sprintf("mba_%d", rewriteIndex)

	builder.SetInsertPointBefore(inst)

	var replacement llvm.Value
	switch inst.InstructionOpcode() {
	case llvm.Add:
		// MBA identity: x + y = (x & y) + (x | y)
		andVal := builder.CreateAnd(left, right, prefix+"_and")
		orVal := builder.CreateOr(left, right, prefix+"_or")
		replacement = builder.CreateAdd(andVal, orVal, prefix+"_res")
	case llvm.Sub:
		// MBA identity: x - y = (x & ~y) - (~x & y)
		notRight := builder.CreateXor(right, allOnes, prefix+"_not_y")
		notLeft := builder.CreateXor(left, allOnes, prefix+"_not_x")
		leftMasked := builder.CreateAnd(left, notRight, prefix+"_x_and_ny")
		rightMasked := builder.CreateAnd(notLeft, right, prefix+"_nx_and_y")
		replacement = builder.CreateSub(leftMasked, rightMasked, prefix+"_res")
	default:
		return fmt.Errorf("mba: unsupported opcode %d", int(inst.InstructionOpcode()))
	}

	inst.ReplaceAllUsesWith(replacement)
	inst.EraseFromParent()
	return nil
}
