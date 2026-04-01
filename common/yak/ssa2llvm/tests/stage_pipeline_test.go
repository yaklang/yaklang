package tests

import (
	"fmt"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
)

const (
	stageProbeName       = "stageprobe"
	stageProbePreTag     = "stageprobe:add1"
	stageProbePostTag    = "stageprobe:add2"
	stageProbePreIRName  = "stageprobe_pre_add"
	stageProbePostIRName = "stageprobe_post_add"
	stageProbeLLVMIRName = "stageprobe_llvm_add_3_3"
)

var registerStageProbeOnce sync.Once

type stageProbeObfuscator struct{}

func registerStageProbeObfuscator() {
	registerStageProbeOnce.Do(func() {
		obfuscation.Register(stageProbeObfuscator{})
	})
}

func (stageProbeObfuscator) Name() string { return stageProbeName }

func (stageProbeObfuscator) Kind() obfuscation.Kind { return obfuscation.KindHybrid }

func (stageProbeObfuscator) Apply(ctx *obfuscation.Context) error {
	switch ctx.Stage {
	case obfuscation.StageSSAPre:
		return stageProbePre(ctx)
	case obfuscation.StageSSAPost:
		return stageProbePost(ctx)
	case obfuscation.StageLLVM:
		return stageProbeLLVM(ctx)
	default:
		return nil
	}
}

func stageProbePre(ctx *obfuscation.Context) error {
	fn, call, ret, err := stageProbeCallAndReturn(ctx)
	if err != nil {
		return err
	}
	if callee, ok := callframe.ResolveDirectCallee(ctx.SSA, fn, call); !ok || callee == nil {
		return fmt.Errorf("stageprobe pre: raw direct call not found")
	}
	if tag := ctx.InstructionTag(call.GetId()); tag != "" {
		return fmt.Errorf("stageprobe pre: expected no call lowering tag before preparation, got %q", tag)
	}
	add, err := emitNeutralAddBefore(fn, ret, call, 1, stageProbePreIRName)
	if err != nil {
		return err
	}
	ctx.SetInstructionTag(add.GetId(), stageProbePreTag)
	return nil
}

func stageProbePost(ctx *obfuscation.Context) error {
	fn, call, ret, err := stageProbeCallAndReturn(ctx)
	if err != nil {
		return err
	}
	if tag := ctx.InstructionTag(call.GetId()); tag != "call:internal" {
		return fmt.Errorf("stageprobe post: expected internal call tag, got %q", tag)
	}
	if !ctxHasTag(ctx, stageProbePreTag) {
		return fmt.Errorf("stageprobe post: missing pre-stage marker tag")
	}
	if !hasNeutralAdd(fn, 1) {
		return fmt.Errorf("stageprobe post: missing pre-stage add marker")
	}
	add, err := emitNeutralAddBefore(fn, ret, call, 2, stageProbePostIRName)
	if err != nil {
		return err
	}
	ctx.SetInstructionTag(add.GetId(), stageProbePostTag)
	return nil
}

func stageProbeLLVM(ctx *obfuscation.Context) error {
	if ctx == nil || ctx.LLVM.C == nil {
		return nil
	}
	ir := ctx.LLVM.String()
	if !ctxHasTag(ctx, stageProbePreTag) {
		return fmt.Errorf("stageprobe llvm: missing pre-stage marker tag")
	}
	if !ctxHasTag(ctx, stageProbePostTag) {
		return fmt.Errorf("stageprobe llvm: missing post-stage marker tag")
	}
	if !regexp.MustCompile(`add i64 [^,\n]+, 1`).MatchString(ir) {
		return fmt.Errorf("stageprobe llvm: missing pre-stage llvm marker")
	}
	if !regexp.MustCompile(`add i64 [^,\n]+, 2`).MatchString(ir) {
		return fmt.Errorf("stageprobe llvm: missing post-stage llvm marker")
	}

	builder := ctx.LLVM.NewBuilder()
	defer builder.Dispose()
	var inserted bool
	three := llvm.ConstInt(ctx.LLVM.Context().Int64Type(), 3, false)
	for function := ctx.LLVM.FirstFunction(); !function.IsNil() && !inserted; function = function.NextFunction() {
		for block := function.FirstBasicBlock(); !block.IsNil() && !inserted; block = block.NextBasicBlock() {
			for inst := block.FirstInstruction(); !inst.IsNil(); inst = inst.NextInstruction() {
				if inst.Type() != ctx.LLVM.Context().Int64Type() {
					continue
				}
				next := inst.NextInstruction()
				if next.IsNil() {
					builder.SetInsertPointAtEnd(block)
				} else {
					builder.SetInsertPointBefore(next)
				}
				builder.CreateAdd(inst, three, stageProbeLLVMIRName)
				inserted = true
				break
			}
		}
	}
	if !inserted {
		return fmt.Errorf("stageprobe llvm: failed to insert llvm marker add")
	}
	return nil
}

func stageProbeCallAndReturn(ctx *obfuscation.Context) (*ssa.Function, *ssa.Call, *ssa.Return, error) {
	if ctx == nil || ctx.SSA == nil {
		return nil, nil, nil, fmt.Errorf("stageprobe: missing ssa program")
	}
	var fn *ssa.Function
	ctx.SSA.EachFunction(func(candidate *ssa.Function) {
		if fn == nil && candidate != nil && candidate.GetName() == "check" {
			fn = candidate
		}
	})
	if fn == nil {
		return nil, nil, nil, fmt.Errorf("stageprobe: function check not found")
	}

	var call *ssa.Call
	var ret *ssa.Return
	for _, blockID := range fn.Blocks {
		blockValue, ok := fn.GetValueById(blockID)
		if !ok || blockValue == nil {
			continue
		}
		block, ok := ssa.ToBasicBlock(blockValue)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok || inst == nil {
				continue
			}
			if candidateRet, ok := ssa.ToReturn(inst); ok && candidateRet != nil {
				ret = candidateRet
			}
			candidateCall, ok := ssa.ToCall(inst)
			if !ok || candidateCall == nil || call != nil {
				continue
			}
			if callee, ok := callframe.ResolveDirectCallee(ctx.SSA, fn, candidateCall); ok && callee != nil && callee.GetName() == "one" {
				call = candidateCall
			}
		}
	}
	if call == nil {
		return fn, nil, nil, fmt.Errorf("stageprobe: target call not found")
	}
	if ret == nil {
		return fn, nil, nil, fmt.Errorf("stageprobe: target return not found")
	}
	return fn, call, ret, nil
}

func emitNeutralAddBefore(fn *ssa.Function, ret *ssa.Return, base ssa.Value, delta int64, name string) (*ssa.BinOp, error) {
	if fn == nil || ret == nil || base == nil {
		return nil, fmt.Errorf("stageprobe: invalid insertion point")
	}
	builder := fn.GetOrCreateBuilder()
	if builder == nil {
		return nil, fmt.Errorf("stageprobe: builder is nil")
	}
	deltaConst := ssa.NewConst(delta)
	builder.EmitInstructionBefore(deltaConst, ret)
	deltaConst.GetProgram().AddConstInstruction(deltaConst)
	add := ssa.NewBinOp(ssa.OpAdd, base, deltaConst)
	add.SetName(name)
	builder.EmitInstructionBefore(add, ret)
	return add, nil
}

func hasNeutralAdd(fn *ssa.Function, delta int64) bool {
	if fn == nil {
		return false
	}
	for _, blockID := range fn.Blocks {
		blockValue, ok := fn.GetValueById(blockID)
		if !ok || blockValue == nil {
			continue
		}
		block, ok := ssa.ToBasicBlock(blockValue)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			inst, ok := fn.GetInstructionById(instID)
			if !ok || inst == nil {
				continue
			}
			binOp, ok := ssa.ToBinOp(inst)
			if !ok || binOp == nil || binOp.Op != ssa.OpAdd {
				continue
			}
			y, ok := fn.GetValueById(binOp.Y)
			if !ok || y == nil {
				continue
			}
			constInst, ok := ssa.ToConstInst(y)
			if !ok || constInst == nil {
				continue
			}
			if raw, ok := constInst.GetRawValue().(int64); ok && raw == delta {
				return true
			}
		}
	}
	return false
}

func ctxHasTag(ctx *obfuscation.Context, want string) bool {
	if ctx == nil || want == "" {
		return false
	}
	for _, tag := range ctx.InstrTags {
		if tag == want {
			return true
		}
	}
	return false
}

func TestStagePipelineRunsPreLowerPostLLVMInOrder(t *testing.T) {
	registerStageProbeObfuscator()

	code := `
one = () => { return 7 }
check = () => {
	return one()
}
`
	ir := CompileLLVMIRString(
		t,
		code,
		"yak",
		compiler.WithCompileObfuscators(stageProbeName),
	)
	require.Regexp(t, regexp.MustCompile(`add i64 [^,\n]+, 1`), ir)
	require.Regexp(t, regexp.MustCompile(`add i64 [^,\n]+, 2`), ir)
	require.Regexp(t, regexp.MustCompile(`stageprobe_llvm_add_3_3 = add i64 [^,\n]+, 3`), ir)
	require.Contains(t, ir, "@one")
}
