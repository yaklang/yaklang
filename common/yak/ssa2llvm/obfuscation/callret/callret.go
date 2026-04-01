package callret

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	intrinsicVSPush = "__yak_obf_vs_push"
	intrinsicVSPop  = "__yak_obf_vs_pop"
	intrinsicCSPush = "__yak_obf_cs_push"
	intrinsicCSPop  = "__yak_obf_cs_pop"

	tagInternalCall = "call:internal"
	tagVSPush       = "callret:vs_push"
	tagVSPop        = "callret:vs_pop"
	tagCSPush       = "callret:cs_push"
	tagCSPop        = "callret:cs_pop"
)

type callRetObfuscator struct{}

func init() {
	core.Register(callRetObfuscator{})
	core.RegisterTaggedCallLowering(tagVSPush, core.TaggedCallLowering{Symbol: intrinsicVSPush, Arity: 1})
	core.RegisterTaggedCallLowering(tagVSPop, core.TaggedCallLowering{Symbol: intrinsicVSPop, Arity: 0})
	core.RegisterTaggedCallLowering(tagCSPush, core.TaggedCallLowering{Symbol: intrinsicCSPush, Arity: 1})
	core.RegisterTaggedCallLowering(tagCSPop, core.TaggedCallLowering{Symbol: intrinsicCSPop, Arity: 0})
}

func (callRetObfuscator) Name() string { return "callret" }

func (callRetObfuscator) Kind() core.Kind { return core.KindHybrid }

func (callRetObfuscator) Apply(ctx *core.Context) error {
	if ctx == nil {
		return nil
	}
	switch ctx.Stage {
	case core.StageSSAPre:
		return applySSAPre(ctx)
	case core.StageSSAPost:
		return applySSAPost(ctx)
	case core.StageLLVM:
		return applyLLVM(ctx)
	default:
		return nil
	}
}

type intrinsicSymbols struct {
	vsPush ssa.Value
	vsPop  ssa.Value
	csPush ssa.Value
	csPop  ssa.Value
}

func applySSAPre(ctx *core.Context) error {
	if ctx == nil || ctx.SSA == nil {
		return nil
	}
	if ctx.SSA.Language != ssaconfig.Yak {
		return nil
	}
	return nil
}

func applySSAPost(ctx *core.Context) error {
	program := ctx.SSA
	if program == nil {
		return nil
	}
	if program.Language != ssaconfig.Yak {
		return nil
	}

	entryFunc, err := findSSAEntryFunction(program, ctx.EntryFunction)
	if err != nil {
		return err
	}

	builder := entryFunc.GetOrCreateBuilder()
	if builder == nil {
		return fmt.Errorf("callret: entry function %q builder is nil", entryFunc.GetName())
	}

	intrinsics, err := ensureIntrinsics(program)
	if err != nil {
		return err
	}

	// Collect only current-program Yak functions. Imported/upstream libraries and
	// runtime/extern targets intentionally stay out of v1 callret.
	internal := make(map[int64]*ssa.Function)
	program.EachFunction(func(fn *ssa.Function) {
		if fn == nil || fn.IsExtern() || fn.GetProgram() != program {
			return
		}
		internal[fn.GetId()] = fn
	})

	// callret is a whole-program rewrite for ordinary Yak internal calls.
	// We first merge every internal callee's basic blocks into the selected
	// entry function, so the later single walk over entryFunc.Blocks still
	// visits instructions that originally lived in other functions.
	// Flatten all functions into the entry function (keep the entry itself).
	// Keep this deterministic by walking ids in ascending order.
	ids := make([]int64, 0, len(internal))
	for id := range internal {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		fn := internal[id]
		if fn == nil || fn == entryFunc {
			continue
		}
		entryFunc.Blocks = append(entryFunc.Blocks, fn.Blocks...)
		if err := rewriteFunctionInputs(ctx, builder, fn, intrinsics.vsPop); err != nil {
			return err
		}
	}

	// Ensure the compiler only emits the obfuscated entry function.
	// Nested functions are compiled via EachFunction recursion on ChildFuncs, so clear them too.
	if program.Funcs != nil {
		var keys []string
		program.Funcs.ForEach(func(name string, _ *ssa.Function) bool {
			keys = append(keys, name)
			return true
		})
		for _, key := range keys {
			program.Funcs.Delete(key)
		}
		program.Funcs.Set(entryFunc.GetName(), entryFunc)
	}
	entryFunc.ChildFuncs = nil

	// The entry function is called by the runtime/host. Keep its ABI params intact.

	// Return dispatch/exit scaffolding (filled later once we know continuation targets).
	retDispatch := entryFunc.NewBasicBlock("obf_ret_dispatch")
	retExit := entryFunc.NewBasicBlock("obf_ret_exit")

	// Build an entry prologue block to push the "final return" continuation id (0) exactly once.
	// This is required for recursion, because internal jumps to the entry function must not push 0 again.
	entryBodyBlock, ok := getBasicBlock(entryFunc, entryFunc.EnterBlock)
	if !ok || entryBodyBlock == nil {
		return fmt.Errorf("callret: entry block not found for %q", entryFunc.GetName())
	}

	entryPrologue := entryFunc.NewBasicBlockNotAddBlocks("obf_entry_prologue")
	if entryPrologue == nil {
		return fmt.Errorf("callret: failed to create entry prologue block")
	}
	entryFunc.EnterBlock = entryPrologue.GetId()
	entryFunc.Blocks = append([]int64{entryPrologue.GetId()}, entryFunc.Blocks...)

	if err := emitCallStackPushConst(ctx, builder, entryPrologue, intrinsics.csPush, 0); err != nil {
		return err
	}
	if err := emitJumpAtBlockEnd(builder, entryPrologue, entryBodyBlock); err != nil {
		return err
	}

	entryFuncID := entryFunc.GetId()

	var continuationBlocks []*ssa.BasicBlock

	// From this point on we intentionally scan only entryFunc.Blocks.
	// Every non-entry internal function body has already been merged into that
	// block list above, and ChildFuncs has been cleared so the compiler will
	// emit only the rewritten entry function.
	// 1) Rewrite direct internal calls into push/pop+jump form, creating continuation blocks.
	for blockIndex := 0; blockIndex < len(entryFunc.Blocks); blockIndex++ {
		blockID := entryFunc.Blocks[blockIndex]
		block, ok := getBasicBlock(entryFunc, blockID)
		if !ok || block == nil {
			continue
		}

		skippedCalls := make(map[int64]struct{})
		for {
			var call *ssa.Call
			var calleeFunc *ssa.Function

			instIDs := append([]int64(nil), block.Insts...)
			for _, instID := range instIDs {
				if _, skip := skippedCalls[instID]; skip {
					continue
				}
				inst, ok := entryFunc.GetInstructionById(instID)
				if !ok || inst == nil {
					continue
				}
				if inst.IsLazy() {
					inst = inst.Self()
				}
				candidate, ok := ssa.ToCall(inst)
				if !ok || candidate == nil {
					continue
				}
				if !isSupportedCallretInvokeCall(ctx, candidate) {
					continue
				}

				resolvedCallee, ok := resolveCallretTargetFunction(program, candidate, internal)
				if !ok || resolvedCallee == nil {
					continue
				}

				call = candidate
				calleeFunc = resolvedCallee
				break
			}
			if call == nil || calleeFunc == nil {
				break
			}

			calleeEnter, ok := getBasicBlock(calleeFunc, calleeFunc.EnterBlock)
			if !ok || calleeEnter == nil {
				return fmt.Errorf("callret: callee %q entry block not found", calleeFunc.GetName())
			}
			if calleeFunc.GetId() == entryFuncID {
				calleeEnter = entryBodyBlock
			}

			cont, err := splitBlockAfter(entryFunc, block, call.GetId())
			if err != nil {
				return err
			}

			regionBlocks := collectReachableBlocks(entryFunc, cont)
			if !valueUsersStayInRegion(call, regionBlocks) {
				// TODO(callret): support call result users that escape the continuation region.
				skippedCalls[call.GetId()] = struct{}{}
				undoSplitBlock(entryFunc, block, cont, call.GetId())
				continue
			}

			liveValues := collectContinuationLiveValues(regionBlocks, call)
			if valueHasPhiUserInRegion(call, regionBlocks) || valuesHavePhiUsersInRegion(liveValues, regionBlocks) {
				// TODO(callret): handle phi-sensitive value restoration for recursive/control-flow-heavy call sites.
				skippedCalls[call.GetId()] = struct{}{}
				undoSplitBlock(entryFunc, block, cont, call.GetId())
				continue
			}
			continuationBlocks = append(continuationBlocks, cont)

			// Always pop one return value to keep the value stack balanced.
			retVal := emitValueStackPopAtBlockStart(ctx, builder, cont, intrinsics.vsPop)
			if retVal == nil {
				return fmt.Errorf("callret: failed to emit valuestack pop in continuation block %d", cont.GetId())
			}
			restoreAfter, ok := retVal.(ssa.Instruction)
			if !ok || restoreAfter == nil {
				return fmt.Errorf("callret: expected return pop to be an instruction (id=%d)", retVal.GetId())
			}

			restoredLiveValues, err := emitContinuationLiveValueRestores(ctx, builder, cont, restoreAfter, intrinsics.vsPop, liveValues)
			if err != nil {
				return err
			}

			replaceValueUsesInRegion(call, retVal, regionBlocks)
			for i, liveValue := range liveValues {
				replaceValueUsesInRegionSkippingPhi(liveValue, restoredLiveValues[i], regionBlocks)
			}

			// Prepare call frame in the original block: push args (reverse order) + push cont id.
			if err := emitValueStackPushValuesBefore(ctx, builder, call, intrinsics.vsPush, liveValues); err != nil {
				return err
			}
			if err := emitValueStackPushInvokeArgs(ctx, builder, call, calleeFunc, intrinsics.vsPush); err != nil {
				return err
			}
			if err := emitCallStackPushConstBefore(ctx, builder, call, intrinsics.csPush, cont.GetId()); err != nil {
				return err
			}

			// Replace the call site with a jump to callee entry.
			if err := emitJumpAtBlockEnd(builder, block, calleeEnter); err != nil {
				return err
			}

			ssa.DeleteInst(call)
		}
	}

	// 2) Rewrite returns into push+dispatch-jump.
	for _, blockID := range entryFunc.Blocks {
		block, ok := getBasicBlock(entryFunc, blockID)
		if !ok || block == nil {
			continue
		}
		instIDs := append([]int64(nil), block.Insts...)
		for _, instID := range instIDs {
			inst, ok := entryFunc.GetInstructionById(instID)
			if !ok || inst == nil {
				continue
			}
			if inst.IsLazy() {
				inst = inst.Self()
			}
			ret, ok := ssa.ToReturn(inst)
			if !ok || ret == nil {
				continue
			}
			if block == retExit {
				// Final exit return is created after the transform pass.
				continue
			}

			if err := rewriteReturn(ctx, builder, ret, intrinsics, retDispatch); err != nil {
				return err
			}
		}
	}

	// 3) Fill the return dispatcher and exit block.
	if err := buildReturnDispatch(ctx, builder, retDispatch, retExit, continuationBlocks, intrinsics.vsPop); err != nil {
		return err
	}
	if err := buildExitBlock(ctx, builder, retExit, intrinsics.vsPop); err != nil {
		return err
	}

	return nil
}

func ensureIntrinsics(program *ssa.Program) (*intrinsicSymbols, error) {
	if program == nil {
		return nil, nil
	}
	vsPush, err := ensureExternUndefined(program, intrinsicVSPush)
	if err != nil {
		return nil, err
	}
	vsPop, err := ensureExternUndefined(program, intrinsicVSPop)
	if err != nil {
		return nil, err
	}
	csPush, err := ensureExternUndefined(program, intrinsicCSPush)
	if err != nil {
		return nil, err
	}
	csPop, err := ensureExternUndefined(program, intrinsicCSPop)
	if err != nil {
		return nil, err
	}
	return &intrinsicSymbols{
		vsPush: vsPush,
		vsPop:  vsPop,
		csPush: csPush,
		csPop:  csPop,
	}, nil
}

func ensureExternUndefined(program *ssa.Program, name string) (ssa.Value, error) {
	if program == nil {
		return nil, nil
	}
	if cached, ok := program.GetCacheExternInstance(name); ok && cached != nil {
		return cached, nil
	}
	un := ssa.NewUndefined(name)
	un.SetExtern(true)
	un.SetProgram(program)
	program.SetVirtualRegister(un)
	program.SetCacheExternInstance(name, un)
	return un, nil
}

func findSSAEntryFunction(program *ssa.Program, requested string) (*ssa.Function, error) {
	if program == nil {
		return nil, fmt.Errorf("callret: nil program")
	}

	candidates := entryFunctionCandidates(requested)

	byName := make(map[string]*ssa.Function, len(candidates))
	program.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		name := fn.GetName()
		if name == "" {
			return
		}
		if _, exists := byName[name]; exists {
			return
		}
		byName[name] = fn
	})

	for _, candidate := range candidates {
		if fn := byName[candidate]; fn != nil {
			return fn, nil
		}
	}
	if strings.TrimSpace(requested) == "" {
		requested = "check"
	}
	return nil, fmt.Errorf("callret: entry function %q not found in SSA program", requested)
}

func entryFunctionCandidates(requested string) []string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "check"
	}

	seen := make(map[string]struct{}, 4)
	add := func(name string, out *[]string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	}

	out := make([]string, 0, 4)
	add(requested, &out)
	if strings.HasPrefix(requested, "@") {
		add(strings.TrimPrefix(requested, "@"), &out)
	} else {
		add("@"+requested, &out)
	}
	if requested == "check" {
		add("main", &out)
		add("@main", &out)
	}
	return out
}

func getBasicBlock(fn *ssa.Function, id int64) (*ssa.BasicBlock, bool) {
	if fn == nil || id <= 0 {
		return nil, false
	}
	v, ok := fn.GetValueById(id)
	if !ok || v == nil {
		return nil, false
	}
	b, ok := ssa.ToBasicBlock(v)
	return b, ok && b != nil
}

func isSupportedCallretInvokeCall(ctx *core.Context, call *ssa.Call) bool {
	if call == nil {
		return false
	}
	if ctx == nil || ctx.InstructionTag(call.GetId()) != tagInternalCall {
		return false
	}
	if call.Async || call.Unpack || call.IsDropError || call.IsEllipsis {
		return false
	}
	return true
}

func resolveCallretTargetFunction(program *ssa.Program, call *ssa.Call, internal map[int64]*ssa.Function) (*ssa.Function, bool) {
	if program == nil || call == nil {
		return nil, false
	}
	callerFn := call.GetFunc()
	resolved, ok := callframe.ResolveDirectCallee(program, callerFn, call)
	if !ok || resolved == nil {
		return nil, false
	}
	if fn, ok := internal[resolved.GetId()]; ok && fn != nil {
		return fn, true
	}
	return resolved, resolved.GetProgram() == program
}

func rewriteFunctionInputs(ctx *core.Context, builder *ssa.FunctionBuilder, fn *ssa.Function, vsPopTarget ssa.Value) error {
	if fn == nil || builder == nil {
		return nil
	}
	entryBlock, ok := getBasicBlock(fn, fn.EnterBlock)
	if !ok || entryBlock == nil {
		return fmt.Errorf("callret: params rewrite missing entry block for %q", fn.GetName())
	}
	if len(fn.Params) == 0 && len(fn.ParameterMembers) == 0 && len(fn.FreeValues) == 0 {
		return nil
	}

	var before ssa.Instruction
	if len(entryBlock.Insts) > 0 {
		first, ok := fn.GetInstructionById(entryBlock.Insts[0])
		if ok {
			before = first
		}
	}

	for _, input := range callframe.OrderedCallFrameInputs(fn) {
		if input.FunctionLike {
			continue
		}
		popCall, err := emitObfPopBefore(ctx, builder, before, entryBlock, vsPopTarget, tagVSPop)
		if err != nil {
			return err
		}
		if input.Value != nil {
			ssa.ReplaceAllValue(input.Value, popCall)
		}
		switch input.Kind {
		case callframe.FrameInputParam:
			fn.Params[input.Index] = popCall.GetId()
		case callframe.FrameInputParamMember:
			fn.ParameterMembers[input.Index] = popCall.GetId()
		case callframe.FrameInputFreeValue:
			fn.FreeValues[input.Variable] = popCall.GetId()
		}
	}
	return nil
}

func collectReachableBlocks(fn *ssa.Function, start *ssa.BasicBlock) map[int64]*ssa.BasicBlock {
	reachable := make(map[int64]*ssa.BasicBlock)
	if fn == nil || start == nil {
		return reachable
	}

	stack := []*ssa.BasicBlock{start}
	for len(stack) > 0 {
		last := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if last == nil {
			continue
		}
		if _, ok := reachable[last.GetId()]; ok {
			continue
		}
		reachable[last.GetId()] = last
		for _, succID := range last.Succs {
			succ, ok := getBasicBlock(fn, succID)
			if ok && succ != nil {
				stack = append(stack, succ)
			}
		}
	}
	return reachable
}

func valueUsersStayInRegion(value ssa.Value, region map[int64]*ssa.BasicBlock) bool {
	if value == nil {
		return true
	}
	for _, user := range value.GetUsers() {
		if user == nil {
			continue
		}
		block := user.GetBlock()
		if block == nil {
			return false
		}
		if _, ok := region[block.GetId()]; !ok {
			return false
		}
	}
	return true
}

func valueHasPhiUserInRegion(value ssa.Value, region map[int64]*ssa.BasicBlock) bool {
	if value == nil {
		return false
	}
	for _, user := range value.GetUsers() {
		if user == nil {
			continue
		}
		inst := ssa.Instruction(user)
		if inst.IsLazy() {
			inst = inst.Self()
		}
		if _, ok := ssa.ToPhi(inst); !ok {
			continue
		}
		block := inst.GetBlock()
		if block == nil {
			return true
		}
		if _, ok := region[block.GetId()]; ok {
			return true
		}
	}
	return false
}

func valuesHavePhiUsersInRegion(values []ssa.Value, region map[int64]*ssa.BasicBlock) bool {
	for _, value := range values {
		if valueHasPhiUserInRegion(value, region) {
			return true
		}
	}
	return false
}

func shouldPreserveLiveValue(value ssa.Value) bool {
	if value == nil || value.GetId() <= 0 {
		return false
	}
	if inst, ok := value.(ssa.Instruction); ok && inst.IsLazy() {
		if self, ok := inst.Self().(ssa.Value); ok && self != nil {
			value = self
		}
	}
	if fn, ok := ssa.ToFunction(value); ok && fn != nil {
		return false
	}
	if param, ok := ssa.ToParameter(value); ok && param != nil {
		if defVal := param.GetDefault(); defVal != nil {
			if fn, ok := ssa.ToFunction(defVal); ok && fn != nil {
				return false
			}
			if _, ok := defVal.GetType().(*ssa.FunctionType); ok {
				return false
			}
		}
	}
	if value.GetOpcode() == ssa.SSAOpcodeFunction {
		return false
	}
	if _, ok := value.GetType().(*ssa.FunctionType); ok {
		return false
	}
	switch value.(type) {
	case *ssa.ConstInst, *ssa.Undefined, *ssa.Function, *ssa.BasicBlock:
		return false
	default:
		return true
	}
}

func collectContinuationLiveValues(region map[int64]*ssa.BasicBlock, triggerCall *ssa.Call) []ssa.Value {
	if len(region) == 0 || triggerCall == nil {
		return nil
	}

	callBlock := triggerCall.GetBlock()
	if callBlock == nil {
		return nil
	}
	callIndex := slices.Index(callBlock.Insts, triggerCall.GetId())
	if callIndex < 0 {
		return nil
	}
	valuesDefinedBeforeCall := make(map[int64]ssa.Value)
	for _, instID := range callBlock.Insts[:callIndex] {
		inst, ok := callBlock.GetInstructionById(instID)
		if !ok || inst == nil {
			continue
		}
		if inst.IsLazy() {
			inst = inst.Self()
		}
		value, ok := inst.(ssa.Value)
		if !ok || !shouldPreserveLiveValue(value) {
			continue
		}
		valuesDefinedBeforeCall[value.GetId()] = value
	}

	liveByID := make(map[int64]ssa.Value)
	for _, block := range region {
		for _, instID := range block.Insts {
			inst, ok := block.GetInstructionById(instID)
			if !ok || inst == nil {
				continue
			}
			if inst.IsLazy() {
				inst = inst.Self()
			}
			user, ok := inst.(interface {
				HasValues() bool
				GetValues() ssa.Values
			})
			if !ok || !user.HasValues() {
				continue
			}
			for _, value := range user.GetValues() {
				if value == nil || value.GetId() == triggerCall.GetId() {
					continue
				}
				candidate, ok := valuesDefinedBeforeCall[value.GetId()]
				if !ok {
					continue
				}
				liveByID[value.GetId()] = candidate
			}
		}
	}

	ids := make([]int64, 0, len(liveByID))
	for id := range liveByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	values := make([]ssa.Value, 0, len(ids))
	for _, id := range ids {
		values = append(values, liveByID[id])
	}
	return values
}

func replaceValueUsesInRegion(from, to ssa.Value, region map[int64]*ssa.BasicBlock) {
	if from == nil || to == nil || len(region) == 0 {
		return
	}
	ssa.ReplaceValue(from, to, func(inst ssa.Instruction) bool {
		if inst == nil {
			return true
		}
		block := inst.GetBlock()
		if block == nil {
			return true
		}
		_, ok := region[block.GetId()]
		return !ok
	})
}

func replaceValueUsesInRegionSkippingPhi(from, to ssa.Value, region map[int64]*ssa.BasicBlock) {
	if from == nil || to == nil || len(region) == 0 {
		return
	}
	ssa.ReplaceValue(from, to, func(inst ssa.Instruction) bool {
		if inst == nil {
			return true
		}
		if inst.IsLazy() {
			inst = inst.Self()
		}
		if _, ok := ssa.ToPhi(inst); ok {
			return true
		}
		block := inst.GetBlock()
		if block == nil {
			return true
		}
		_, ok := region[block.GetId()]
		return !ok
	})
}

func undoSplitBlock(fn *ssa.Function, block, cont *ssa.BasicBlock, splitInstID int64) {
	if fn == nil || block == nil || cont == nil {
		return
	}

	idx := slices.Index(fn.Blocks, cont.GetId())
	if idx >= 0 {
		fn.Blocks = append(fn.Blocks[:idx], fn.Blocks[idx+1:]...)
	}

	block.Insts = append(block.Insts, cont.Insts...)
	for _, movedID := range cont.Insts {
		movedInst, ok := fn.GetInstructionById(movedID)
		if !ok || movedInst == nil {
			continue
		}
		if movedInst.IsLazy() {
			movedInst = movedInst.Self()
		}
		movedInst.SetBlock(block)
	}

	block.Succs = append([]int64(nil), cont.Succs...)
	for _, succID := range cont.Succs {
		succ, ok := getBasicBlock(fn, succID)
		if !ok || succ == nil {
			continue
		}
		for i, predID := range succ.Preds {
			if predID == cont.GetId() {
				succ.Preds[i] = block.GetId()
			}
		}
	}

	fn.GetProgram().DeleteInstruction(cont)
	_ = splitInstID
}

func splitBlockAfter(fn *ssa.Function, block *ssa.BasicBlock, instID int64) (*ssa.BasicBlock, error) {
	if fn == nil || block == nil {
		return nil, fmt.Errorf("callret: splitBlockAfter nil inputs")
	}
	idx := slices.Index(block.Insts, instID)
	if idx < 0 {
		return nil, fmt.Errorf("callret: instruction %d not found in block %d", instID, block.GetId())
	}

	cont := fn.NewBasicBlock("obf_cont")
	tail := append([]int64(nil), block.Insts[idx+1:]...)
	block.Insts = block.Insts[:idx+1]

	cont.Insts = tail
	for _, movedID := range tail {
		movedInst, ok := fn.GetInstructionById(movedID)
		if !ok || movedInst == nil {
			continue
		}
		if movedInst.IsLazy() {
			movedInst = movedInst.Self()
		}
		movedInst.SetBlock(cont)
	}

	oldSuccs := append([]int64(nil), block.Succs...)
	block.Succs = nil
	cont.Succs = append([]int64(nil), oldSuccs...)
	for _, succID := range oldSuccs {
		succ, ok := getBasicBlock(fn, succID)
		if !ok || succ == nil {
			continue
		}
		for i, predID := range succ.Preds {
			if predID == block.GetId() {
				succ.Preds[i] = cont.GetId()
			}
		}
	}

	return cont, nil
}

func emitCallStackPushConst(ctx *core.Context, builder *ssa.FunctionBuilder, block *ssa.BasicBlock, csPushTarget ssa.Value, raw int64) error {
	if builder == nil || block == nil {
		return nil
	}

	var before ssa.Instruction
	if len(block.Insts) > 0 {
		first, ok := block.GetInstructionById(block.Insts[0])
		if ok {
			before = first
		}
	}

	c := ssa.NewConst(raw)
	if before != nil {
		builder.EmitInstructionBefore(c, before)
	} else {
		builder.CurrentBlock = block
		builder.Function = block.GetFunc()
		builder.EmitFirst(c, block)
	}
	c.GetProgram().AddConstInstruction(c)

	push := newObfIntrinsicCall(csPushTarget, ssa.Values{c}, block)
	if before != nil {
		builder.EmitInstructionBefore(push, before)
	} else {
		builder.CurrentBlock = block
		builder.Function = block.GetFunc()
		builder.EmitCall(push)
	}
	tagObfCall(ctx, push, tagCSPush)
	return nil
}

func emitCallStackPushConstBefore(ctx *core.Context, builder *ssa.FunctionBuilder, before ssa.Instruction, csPushTarget ssa.Value, raw int64) error {
	if builder == nil || before == nil {
		return nil
	}
	block := before.GetBlock()
	if block == nil {
		return fmt.Errorf("callret: before instruction has nil block")
	}

	c := ssa.NewConst(raw)
	builder.EmitInstructionBefore(c, before)
	c.GetProgram().AddConstInstruction(c)

	push := newObfIntrinsicCall(csPushTarget, ssa.Values{c}, block)
	builder.EmitInstructionBefore(push, before)
	tagObfCall(ctx, push, tagCSPush)
	return nil
}

func emitValueStackPushInvokeArgs(ctx *core.Context, builder *ssa.FunctionBuilder, call *ssa.Call, calleeFn *ssa.Function, vsPushTarget ssa.Value) error {
	if builder == nil || call == nil {
		return nil
	}
	fn := call.GetFunc()
	if fn == nil {
		return fmt.Errorf("callret: call has nil func")
	}

	frameInputs := callframe.OrderedCallFrameInputs(calleeFn)
	argIDs := callframe.BuildCallFrameArgIDs(fn.GetProgram(), call, calleeFn)
	if len(frameInputs) == 0 || len(argIDs) == 0 {
		return nil
	}
	if len(argIDs) < len(frameInputs) {
		return fmt.Errorf("callret: invoke args len %d smaller than frame input len %d for %q", len(argIDs), len(frameInputs), calleeFn.GetName())
	}

	for i := len(frameInputs) - 1; i >= 0; i-- {
		if frameInputs[i].FunctionLike {
			continue
		}
		argID := argIDs[i]
		argVal, ok := fn.GetValueById(argID)
		if !ok || argVal == nil {
			return fmt.Errorf("callret: arg %d (id=%d) not found for %q", i, argID, calleeFn.GetName())
		}
		push := newObfIntrinsicCall(vsPushTarget, ssa.Values{argVal}, call.GetBlock())
		builder.EmitInstructionBefore(push, call)
		tagObfCall(ctx, push, tagVSPush)
	}
	return nil
}

func emitValueStackPopAtBlockStart(ctx *core.Context, builder *ssa.FunctionBuilder, block *ssa.BasicBlock, vsPopTarget ssa.Value) ssa.Value {
	if builder == nil || block == nil {
		return nil
	}
	pop := newObfIntrinsicCall(vsPopTarget, nil, block)
	if len(block.Insts) == 0 {
		builder.CurrentBlock = block
		builder.Function = block.GetFunc()
		builder.EmitCall(pop)
		tagObfCall(ctx, pop, tagVSPop)
		return pop
	}
	first, ok := block.GetInstructionById(block.Insts[0])
	if !ok || first == nil {
		builder.CurrentBlock = block
		builder.Function = block.GetFunc()
		builder.EmitCall(pop)
		tagObfCall(ctx, pop, tagVSPop)
		return pop
	}
	builder.EmitInstructionBefore(pop, first)
	tagObfCall(ctx, pop, tagVSPop)
	return pop
}

func emitValueStackPopAfterLeadingPops(ctx *core.Context, builder *ssa.FunctionBuilder, block *ssa.BasicBlock, vsPopTarget ssa.Value) ssa.Value {
	if builder == nil || block == nil || vsPopTarget == nil {
		return nil
	}

	pop := newObfIntrinsicCall(vsPopTarget, nil, block)

	var after ssa.Instruction
	for _, instID := range block.Insts {
		inst, ok := block.GetInstructionById(instID)
		if !ok || inst == nil {
			break
		}
		if inst.IsLazy() {
			inst = inst.Self()
		}
		call, ok := ssa.ToCall(inst)
		if !ok || call == nil {
			break
		}
		if ctx.InstructionTag(call.GetId()) != tagVSPop {
			break
		}
		after = call
	}

	if after == nil {
		if len(block.Insts) == 0 {
			builder.CurrentBlock = block
			builder.Function = block.GetFunc()
			builder.EmitCall(pop)
			tagObfCall(ctx, pop, tagVSPop)
			return pop
		}
		first, ok := block.GetInstructionById(block.Insts[0])
		if !ok || first == nil {
			builder.CurrentBlock = block
			builder.Function = block.GetFunc()
			builder.EmitCall(pop)
			tagObfCall(ctx, pop, tagVSPop)
			return pop
		}
		builder.EmitInstructionBefore(pop, first)
		tagObfCall(ctx, pop, tagVSPop)
		return pop
	}

	builder.EmitInstructionAfter(pop, after)
	tagObfCall(ctx, pop, tagVSPop)
	return pop
}

func emitValueStackPushValuesBefore(ctx *core.Context, builder *ssa.FunctionBuilder, before ssa.Instruction, vsPushTarget ssa.Value, values []ssa.Value) error {
	if builder == nil || before == nil || len(values) == 0 {
		return nil
	}
	block := before.GetBlock()
	if block == nil {
		return fmt.Errorf("callret: before instruction has nil block")
	}
	for i := len(values) - 1; i >= 0; i-- {
		value := values[i]
		if value == nil {
			continue
		}
		push := newObfIntrinsicCall(vsPushTarget, ssa.Values{value}, block)
		builder.EmitInstructionBefore(push, before)
		tagObfCall(ctx, push, tagVSPush)
	}
	return nil
}

func emitContinuationLiveValueRestores(ctx *core.Context, builder *ssa.FunctionBuilder, block *ssa.BasicBlock, after ssa.Instruction, vsPopTarget ssa.Value, values []ssa.Value) ([]ssa.Value, error) {
	if builder == nil || block == nil || after == nil || len(values) == 0 {
		return nil, nil
	}
	restored := make([]ssa.Value, 0, len(values))
	insertAfter := after
	for range values {
		pop := newObfIntrinsicCall(vsPopTarget, nil, block)
		builder.EmitInstructionAfter(pop, insertAfter)
		tagObfCall(ctx, pop, tagVSPop)
		restored = append(restored, pop)
		insertAfter = pop
	}
	return restored, nil
}

func emitObfPopBefore(ctx *core.Context, builder *ssa.FunctionBuilder, before ssa.Instruction, block *ssa.BasicBlock, target ssa.Value, tag string) (*ssa.Call, error) {
	if builder == nil || target == nil {
		return nil, nil
	}
	pop := newObfIntrinsicCall(target, nil, block)
	if before != nil {
		builder.EmitInstructionBefore(pop, before)
		tagObfCall(ctx, pop, tag)
		return pop, nil
	}
	builder.CurrentBlock = block
	builder.Function = block.GetFunc()
	builder.EmitCall(pop)
	tagObfCall(ctx, pop, tag)
	return pop, nil
}

func newObfIntrinsicCall(target ssa.Value, args ssa.Values, block *ssa.BasicBlock) *ssa.Call {
	return ssa.NewCall(target, args, nil, block)
}

func tagObfCall(ctx *core.Context, call *ssa.Call, tag string) {
	if ctx == nil || call == nil || tag == "" {
		return
	}
	ctx.SetInstructionTag(call.GetId(), tag)
}

func rewriteReturn(ctx *core.Context, builder *ssa.FunctionBuilder, ret *ssa.Return, intrinsics *intrinsicSymbols, retDispatch *ssa.BasicBlock) error {
	if builder == nil || ret == nil || intrinsics == nil || retDispatch == nil {
		return nil
	}

	block := ret.GetBlock()
	if block == nil {
		return fmt.Errorf("callret: return has nil block")
	}

	fn := ret.GetFunc()
	if fn == nil {
		return fmt.Errorf("callret: return has nil func")
	}

	var resultVal ssa.Value
	if len(ret.Results) > 0 {
		v, ok := fn.GetValueById(ret.Results[0])
		if ok {
			resultVal = v
		}
	}
	if resultVal == nil {
		c := ssa.NewConst(int64(0))
		builder.EmitInstructionBefore(c, ret)
		c.GetProgram().AddConstInstruction(c)
		resultVal = c
	}

	vsPushResult := newObfIntrinsicCall(intrinsics.vsPush, ssa.Values{resultVal}, block)
	builder.EmitInstructionBefore(vsPushResult, ret)
	tagObfCall(ctx, vsPushResult, tagVSPush)

	csPop := newObfIntrinsicCall(intrinsics.csPop, nil, block)
	builder.EmitInstructionBefore(csPop, ret)
	tagObfCall(ctx, csPop, tagCSPop)

	vsPushRetID := newObfIntrinsicCall(intrinsics.vsPush, ssa.Values{csPop}, block)
	builder.EmitInstructionBefore(vsPushRetID, ret)
	tagObfCall(ctx, vsPushRetID, tagVSPush)

	ssa.DeleteInst(ret)
	if err := emitJumpAtBlockEnd(builder, block, retDispatch); err != nil {
		return err
	}
	return nil
}

func buildReturnDispatch(ctx *core.Context, builder *ssa.FunctionBuilder, dispatchStart, exitBlock *ssa.BasicBlock, continuations []*ssa.BasicBlock, vsPopTarget ssa.Value) error {
	if builder == nil || dispatchStart == nil || exitBlock == nil {
		return nil
	}

	// retID = vs_pop()
	retID := emitValueStackPopAtBlockStart(ctx, builder, dispatchStart, vsPopTarget)
	if retID == nil {
		return fmt.Errorf("callret: failed to emit retID pop")
	}

	current := dispatchStart
	emitIf := func(block *ssa.BasicBlock, cond ssa.Value, t, f *ssa.BasicBlock) error {
		if block == nil || cond == nil || t == nil || f == nil {
			return fmt.Errorf("callret: invalid if emission")
		}
		builder.CurrentBlock = block
		builder.Function = block.GetFunc()
		ifInst := builder.EmitIf()
		if ifInst == nil {
			return fmt.Errorf("callret: failed to emit if")
		}
		ifInst.Cond = cond.GetId()
		ifInst.True = t.GetId()
		ifInst.False = f.GetId()
		block.AddSucc(t)
		block.AddSucc(f)
		return nil
	}

	// First: retID == 0 => exit.
	constZero := ssa.NewConst(int64(0))
	builder.EmitInstructionAfter(constZero, retID)
	constZero.GetProgram().AddConstInstruction(constZero)

	builder.CurrentBlock = current
	builder.Function = current.GetFunc()
	eqExit := builder.EmitBinOp(ssa.OpEq, retID, constZero)

	next := dispatchStart.GetFunc().NewBasicBlock("obf_ret_dispatch_next")
	if err := emitIf(current, eqExit, exitBlock, next); err != nil {
		return err
	}
	current = next

	// Chain continuations by comparing with their block ids.
	for _, cont := range continuations {
		if cont == nil {
			continue
		}

		c := ssa.NewConst(cont.GetId())
		if len(current.Insts) == 0 {
			builder.CurrentBlock = current
			builder.Function = current.GetFunc()
			builder.EmitFirst(c, current)
		} else {
			first, _ := current.GetInstructionById(current.Insts[0])
			if first != nil {
				builder.EmitInstructionBefore(c, first)
			}
		}
		c.GetProgram().AddConstInstruction(c)

		builder.CurrentBlock = current
		builder.Function = current.GetFunc()
		eq := builder.EmitBinOp(ssa.OpEq, retID, c)

		fallback := dispatchStart.GetFunc().NewBasicBlock("obf_ret_dispatch_next")
		if err := emitIf(current, eq, cont, fallback); err != nil {
			return err
		}
		current = fallback
	}

	// Fallback: discard one return value (if present) and return 0.
	builder.CurrentBlock = current
	builder.Function = current.GetFunc()
	_ = emitValueStackPopAtBlockStart(ctx, builder, current, vsPopTarget)
	if err := emitJumpAtBlockEnd(builder, current, exitBlock); err != nil {
		return err
	}

	return nil
}

func buildExitBlock(ctx *core.Context, builder *ssa.FunctionBuilder, exitBlock *ssa.BasicBlock, vsPopTarget ssa.Value) error {
	if builder == nil || exitBlock == nil {
		return nil
	}
	retVal := emitValueStackPopAtBlockStart(ctx, builder, exitBlock, vsPopTarget)
	if retVal == nil {
		return fmt.Errorf("callret: failed to pop exit return value")
	}
	builder.CurrentBlock = exitBlock
	builder.Function = exitBlock.GetFunc()
	builder.EmitReturn([]ssa.Value{retVal})
	return nil
}

func emitJumpAtBlockEnd(builder *ssa.FunctionBuilder, from *ssa.BasicBlock, to *ssa.BasicBlock) error {
	if builder == nil || from == nil || to == nil {
		return nil
	}

	j := ssa.NewJump(to)

	if len(from.Insts) == 0 {
		// Manual emit to bypass BasicBlock.finish checks in builder.EmitJump.
		j.SetBlock(from)
		j.SetFunc(from.GetFunc())
		j.GetProgram().SetVirtualRegister(j)
		from.Insts = append(from.Insts, j.GetId())
	} else {
		lastID := from.Insts[len(from.Insts)-1]
		lastInst, ok := from.GetInstructionById(lastID)
		if !ok || lastInst == nil {
			j.SetBlock(from)
			j.SetFunc(from.GetFunc())
			j.GetProgram().SetVirtualRegister(j)
			from.Insts = append(from.Insts, j.GetId())
		} else {
			builder.EmitInstructionAfter(j, lastInst)
		}
	}

	from.AddSucc(to)
	return nil
}
