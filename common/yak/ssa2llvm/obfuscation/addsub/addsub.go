package addsub

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

type addSubSSAObfuscator struct{}

func init() {
	core.Register(addSubSSAObfuscator{})
}

func (addSubSSAObfuscator) Name() string {
	return "addsub"
}

func (addSubSSAObfuscator) Kind() core.Kind {
	return core.KindSSA
}

// Run rewrites direct SSA add/sub instructions into equivalent three-node forms.
//
// The local SSA rewrites are:
//
//	add(x, y) => add(sub(x, k), add(y, k))
//	sub(x, y) => sub(add(x, k), add(y, k))
//
// k is derived from the original instruction id, so the pass is deterministic
// for the same SSA graph while still breaking the original arithmetic shape.
func (addSubSSAObfuscator) Apply(ctx *core.Context) error {
	if ctx == nil || ctx.Stage != core.StageSSAPre {
		return nil
	}
	program := ctx.SSA
	if program == nil {
		return nil
	}

	var runErr error
	program.EachFunction(func(fn *ssa.Function) {
		if runErr != nil || fn == nil {
			return
		}
		builder := fn.GetOrCreateBuilder()
		if builder == nil {
			runErr = fmt.Errorf("function %q builder is nil", fn.GetName())
			return
		}

		blockIDs := append([]int64(nil), fn.Blocks...)
		for _, blockID := range blockIDs {
			blockValue, ok := fn.GetValueById(blockID)
			if !ok || blockValue == nil {
				continue
			}
			block, ok := ssa.ToBasicBlock(blockValue)
			if !ok || block == nil {
				continue
			}

			instIDs := append([]int64(nil), block.Insts...)
			for _, instID := range instIDs {
				inst, ok := fn.GetInstructionById(instID)
				if !ok || inst == nil {
					continue
				}
				binOp, ok := ssa.ToBinOp(inst)
				if !ok || binOp == nil {
					continue
				}
				if binOp.Op != ssa.OpAdd && binOp.Op != ssa.OpSub {
					continue
				}

				replacement, err := rewriteAddSub(builder, binOp)
				if err != nil {
					runErr = err
					return
				}
				ssa.ReplaceAllValue(binOp, replacement)
				ssa.DeleteInst(binOp)
			}
		}
	})
	return runErr
}

// rewriteAddSub performs the local SSA replacement around a single binary add/sub.
func rewriteAddSub(builder *ssa.FunctionBuilder, original *ssa.BinOp) (ssa.Value, error) {
	leftValue, ok := original.GetValueById(original.X)
	if !ok || leftValue == nil {
		return nil, fmt.Errorf("binop %d left operand not found", original.GetId())
	}
	rightValue, ok := original.GetValueById(original.Y)
	if !ok || rightValue == nil {
		return nil, fmt.Errorf("binop %d right operand not found", original.GetId())
	}

	seed := int64(original.GetId()%97 + 1)
	seedConst := insertConstBefore(builder, original, seed)

	switch original.Op {
	case ssa.OpAdd:
		leftHidden := insertBinOpBefore(builder, original, ssa.OpSub, leftValue, seedConst)
		rightHidden := insertBinOpBefore(builder, original, ssa.OpAdd, rightValue, seedConst)
		return insertBinOpBefore(builder, original, ssa.OpAdd, leftHidden, rightHidden), nil
	case ssa.OpSub:
		leftHidden := insertBinOpBefore(builder, original, ssa.OpAdd, leftValue, seedConst)
		rightHidden := insertBinOpBefore(builder, original, ssa.OpAdd, rightValue, seedConst)
		return insertBinOpBefore(builder, original, ssa.OpSub, leftHidden, rightHidden), nil
	default:
		return nil, fmt.Errorf("unsupported addsub opcode %q", original.Op)
	}
}

func insertConstBefore(builder *ssa.FunctionBuilder, before ssa.Instruction, raw any) *ssa.ConstInst {
	constInst := ssa.NewConst(raw)
	builder.EmitInstructionBefore(constInst, before)
	constInst.GetProgram().AddConstInstruction(constInst)
	return constInst
}

func insertBinOpBefore(builder *ssa.FunctionBuilder, before ssa.Instruction, op ssa.BinaryOpcode, left, right ssa.Value) *ssa.BinOp {
	binOp := ssa.NewBinOp(op, left, right)
	builder.EmitInstructionBefore(binOp, before)
	return binOp
}
