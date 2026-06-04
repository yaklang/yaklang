package compiler

import (
	"fmt"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// compileJump creates an unconditional branch to the target block.
func (c *Compiler) compileJump(inst *ssa.Jump) error {
	if info := c.switchHandlerForJump(inst); info != nil {
		return c.compileSwitchHandlerJump(inst, info)
	}
	targetBlock, ok := c.Blocks[inst.To]
	if !ok {
		return fmt.Errorf("compileJump: target block %d not found", inst.To)
	}
	c.Builder.CreateBr(targetBlock)
	return nil
}

func (c *Compiler) switchHandlerForJump(inst *ssa.Jump) *switchHandlerInfo {
	if c == nil || c.function == nil || inst == nil || inst.GetBlock() == nil || c.function.switchHandlers == nil {
		return nil
	}
	info := c.function.switchHandlers[inst.GetBlock().GetId()]
	if info == nil || info.condID <= 0 || len(info.labelIDs) == 0 || info.trueBlockID <= 0 || info.falseBlockID <= 0 {
		return nil
	}
	return info
}

func (c *Compiler) compileSwitchHandlerJump(inst *ssa.Jump, info *switchHandlerInfo) error {
	trueBlock, ok := c.Blocks[info.trueBlockID]
	if !ok || trueBlock.IsNil() {
		return fmt.Errorf("compileSwitchHandlerJump: true block %d not found", info.trueBlockID)
	}
	falseBlock, ok := c.Blocks[info.falseBlockID]
	if !ok || falseBlock.IsNil() {
		return fmt.Errorf("compileSwitchHandlerJump: false block %d not found", info.falseBlockID)
	}
	match, err := c.emitSwitchLabelMatch(inst, info.condID, info.labelIDs)
	if err != nil {
		return err
	}
	c.Builder.CreateCondBr(match, trueBlock, falseBlock)
	return nil
}

func (c *Compiler) emitSwitchLabelMatch(contextInst ssa.Instruction, condID int64, labelIDs []int64) (llvm.Value, error) {
	if len(labelIDs) == 0 {
		return llvm.ConstInt(c.LLVMCtx.Int1Type(), 0, false), nil
	}
	var combined llvm.Value
	for _, labelID := range labelIDs {
		if labelID <= 0 {
			continue
		}
		match, err := c.emitSwitchLabelEqual(contextInst, condID, labelID)
		if err != nil {
			return llvm.Value{}, err
		}
		if combined.IsNil() {
			combined = match
			continue
		}
		combined = c.Builder.CreateOr(combined, match, "switch_case_any")
	}
	if combined.IsNil() {
		return llvm.ConstInt(c.LLVMCtx.Int1Type(), 0, false), nil
	}
	return combined, nil
}

func (c *Compiler) emitSwitchLabelEqual(contextInst ssa.Instruction, condID, labelID int64) (llvm.Value, error) {
	lhs, lhsRoot, err := c.resolveContextCallArg(contextInst, condID, true)
	if err != nil {
		return llvm.Value{}, err
	}
	rhs, rhsRoot, err := c.resolveContextCallArg(contextInst, labelID, true)
	if err != nil {
		return llvm.Value{}, err
	}
	spec := contextCallSpec{
		inst:      nilResultInstruction{Instruction: contextInst},
		kind:      abi.KindDispatch,
		target:    llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(abi.IDRuntimeEq), false),
		args:      []contextCallArg{{value: lhs, root: lhsRoot}, {value: rhs, root: rhsRoot}, {value: llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)}},
		async:     false,
		ctxName:   "yak_switch_eq_ctx",
		errPrefix: "emitSwitchLabelEqual",
	}
	result, err := c.emitContextCall(spec)
	if err != nil {
		return llvm.Value{}, err
	}
	return c.coerceToI1(result, "switch_case_match"), nil
}

type nilResultInstruction struct {
	ssa.Instruction
}

func (n nilResultInstruction) GetId() int64 {
	return 0
}

// compileIf creates a conditional branch.
// It handles conversion of the condition value to i1 if necessary.
func (c *Compiler) compileIf(inst *ssa.If) error {
	condVal, err := c.getValue(inst, inst.Cond)
	if err != nil {
		return err
	}

	condVal = c.coerceToI1(condVal, "if_cond")

	trueBlock, ok := c.Blocks[inst.True]
	if !ok {
		return fmt.Errorf("compileIf: true block %d not found", inst.True)
	}

	falseBlock, ok := c.Blocks[inst.False]
	if !ok {
		return fmt.Errorf("compileIf: false block %d not found", inst.False)
	}

	c.Builder.CreateCondBr(condVal, trueBlock, falseBlock)
	return nil
}

// compileLoop creates a conditional branch for loop headers.
// It branches to Body when the condition is true, otherwise to Exit.
func (c *Compiler) compileLoop(inst *ssa.Loop) error {
	condVal, err := c.getValue(inst, inst.Cond)
	if err != nil {
		return err
	}

	condVal = c.coerceToI1(condVal, "loop_cond")

	bodyBlock, ok := c.Blocks[inst.Body]
	if !ok {
		return fmt.Errorf("compileLoop: body block %d not found", inst.Body)
	}
	exitBlock, ok := c.Blocks[inst.Exit]
	if !ok {
		return fmt.Errorf("compileLoop: exit block %d not found", inst.Exit)
	}

	c.Builder.CreateCondBr(condVal, bodyBlock, exitBlock)
	return nil
}

func (c *Compiler) compileSwitch(inst *ssa.Switch) error {
	if inst == nil {
		return fmt.Errorf("compileSwitch: nil switch")
	}
	defaultBlock := llvm.BasicBlock{}
	if inst.DefaultBlock != nil {
		defaultBlock = c.Blocks[inst.DefaultBlock.GetId()]
	}
	if defaultBlock.IsNil() {
		return fmt.Errorf("compileSwitch: default block not found")
	}
	for _, label := range inst.Label {
		if label.Dest > 0 {
			caseBlock, ok := c.Blocks[label.Dest]
			if !ok || caseBlock.IsNil() {
				return fmt.Errorf("compileSwitch: case block %d not found", label.Dest)
			}
			c.Builder.CreateBr(caseBlock)
			return nil
		}
	}
	c.Builder.CreateBr(defaultBlock)
	return nil
}

// compilePhi reserves an entry alloca slot for the phi. Incoming edge values are
// stored by resolvePhi; uses load from the slot (mem2reg-friendly).
func (c *Compiler) compilePhi(inst *ssa.Phi) error {
	if inst == nil {
		return fmt.Errorf("compilePhi: nil phi")
	}
	c.ensureValueSlot(inst.GetId())
	if inst.IsMember() && inst.GetObject() != nil && inst.GetKey() != nil {
		c.queuePendingMemberSet(inst, inst.GetId(), inst.GetObject(), inst.GetKey(), true)
	}
	return nil
}

func (c *Compiler) ensurePhiNode(phi *ssa.Phi) error {
	if phi == nil {
		return fmt.Errorf("ensurePhiNode: nil phi")
	}
	c.ensureValueSlot(phi.GetId())
	return nil
}

func (c *Compiler) llvmValueTypeForSSA(t ssa.Type) llvm.Type {
	// SSA2LLVM currently uses a single `i64` value representation for all YakSSA values.
	// Heap/stack addresses are carried as uintptr/i64 and cast to pointers only when
	// calling runtime/extern helpers.
	return c.LLVMCtx.Int64Type()
}

func (c *Compiler) inferPhiType(inst *ssa.Phi) llvm.Type {
	return c.llvmValueTypeForSSA(inst.GetType())
}

func (c *Compiler) ssaValueIsPointer(val ssa.Value, fn *ssa.Function) bool {
	if val == nil {
		return false
	}

	if t := val.GetType(); t != nil {
		switch t.GetTypeKind() {
		case ssa.ObjectTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind, ssa.PointerKind, ssa.StringTypeKind, ssa.FunctionTypeKind:
			return true
		case ssa.StructTypeKind:
			return true
		}
	}

	switch v := val.(type) {
	case *ssa.ConstInst:
		return v.IsString()
	case *ssa.Make:
		return true
	case *ssa.Call:
		return c.callReturnsPointer(v, fn)
	}

	return false
}

func (c *Compiler) callReturnsPointer(call *ssa.Call, fn *ssa.Function) bool {
	if call == nil {
		return false
	}

	calleeName := c.resolveCalleeName(fn, call.Method)

	if binding, ok := c.getExternBinding(calleeName); ok {
		switch binding.Return {
		case ExternTypePtr:
			return true
		default:
			return false
		}
	}

	switch calleeName {
	case "malloc":
		return true
	default:
		return false
	}
}

// resolvePhi stores incoming edge values into each predecessor's outgoing slot store.
func (c *Compiler) resolvePhi(inst *ssa.Phi) error {
	if inst == nil {
		return fmt.Errorf("resolvePhi: nil phi")
	}
	phiID := inst.GetId()
	c.ensureValueSlot(phiID)

	block := inst.GetBlock()
	if block == nil {
		return fmt.Errorf("resolvePhi: phi %d has no block", phiID)
	}
	blockID := block.GetId()

	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("resolvePhi: function for phi %d is nil", phiID)
	}

	blockVal, ok := fn.GetValueById(blockID)
	if !ok {
		return fmt.Errorf("resolvePhi: block %d not found in function", blockID)
	}

	bbSsa, ok := blockVal.(*ssa.BasicBlock)
	if !ok {
		return fmt.Errorf("resolvePhi: value %d is not *ssa.BasicBlock", blockID)
	}

	edges := inst.Edge
	preds := bbSsa.Preds
	if len(preds) == 0 {
		preds = predecessorBlockIDs(fn, blockID)
	}

	edgeByPred := make(map[int64]int64, len(preds))
	for i, predBlockID := range preds {
		if i < len(edges) {
			edgeByPred[predBlockID] = edges[i]
		}
	}

	phiBB, ok := c.Blocks[blockID]
	if !ok || phiBB.IsNil() {
		return fmt.Errorf("resolvePhi: llvm block %d not found", blockID)
	}

	zero := llvm.ConstInt(c.inferPhiType(inst), 0, false)

	prevActive := int64(0)
	if c.function != nil {
		prevActive = c.function.activeBlockID
	}
	defer func() {
		if c.function != nil {
			c.function.activeBlockID = prevActive
		}
	}()

	resolveEdge := func(predBlockID int64, edgeValID int64) llvm.Value {
		if edgeValID <= 0 {
			return zero
		}
		edgeObj, ok := fn.GetValueById(edgeValID)
		if !ok || edgeObj == nil {
			return zero
		}
		if undef, ok := edgeObj.(*ssa.Undefined); ok && undef != nil {
			switch undef.Kind {
			case ssa.UndefinedValueInValid, ssa.UndefinedMemberInValid:
				return zero
			}
		}
		resolved, err := c.resolvePhiIncomingValue(inst, fn, predBlockID, edgeValID)
		if err != nil || resolved.IsNil() {
			return zero
		}
		return resolved
	}

	emitPredStore := func(predBlockID int64, edgeValID int64) {
		predBB, ok := c.Blocks[predBlockID]
		if !ok || predBB.IsNil() {
			return
		}
		c.setInsertPointBeforeTerminator(predBB)
		if c.function != nil {
			c.function.activeBlockID = predBlockID
		}
		resolved := resolveEdge(predBlockID, edgeValID)
		c.setInsertPointBeforeTerminator(predBB)
		if c.function != nil {
			c.function.activeBlockID = predBlockID
		}
		c.storeSSAValue(phiID, resolved)
	}

	emitMemberSet := func() error {
		c.setInsertPointBeforeTerminator(phiBB)
		if c.function != nil {
			c.function.activeBlockID = blockID
		}
		if err := c.maybeEmitMemberSet(inst, inst, phiID); err != nil {
			return err
		}
		if inst.IsMember() && inst.GetObject() != nil {
			if objInst, ok := inst.GetObject().(ssa.Instruction); ok && objInst != nil {
				return c.withInstructionInsertPoint(objInst, func() error {
					c.emitDirectMemberValueSetIfReady(objInst, inst, phiID)
					return c.flushPendingMemberSets(objInst, inst.GetObject(), inst.GetKey())
				})
			}
		}
		return nil
	}

	// Prefer SSA predecessor order (aligned with phi edges) over LLVM CFG discovery.
	if len(preds) > 0 {
		for i, predBlockID := range preds {
			edgeValID := edgeByPred[predBlockID]
			if edgeValID == 0 && i < len(edges) {
				edgeValID = edges[i]
			}
			emitPredStore(predBlockID, edgeValID)
		}
		return emitMemberSet()
	}

	llvmPreds := c.gatherLLVMPredecessors(phiBB)
	for _, predBB := range llvmPreds {
		predID := c.blockIDForLLVM(predBB)
		emitPredStore(predID, edgeByPred[predID])
	}

	return emitMemberSet()
}

func (c *Compiler) resolvePhiIncomingValue(contextInst *ssa.Phi, fn *ssa.Function, predID int64, edgeValID int64) (llvm.Value, error) {
	zero := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	if fn == nil || edgeValID <= 0 {
		return zero, nil
	}
	edgeObj, ok := fn.GetValueById(edgeValID)
	if !ok || edgeObj == nil {
		return zero, nil
	}
	if sideEffect, ok := edgeObj.(*ssa.SideEffect); ok && sideEffect != nil && sideEffect.GetBlock() != nil &&
		sideEffect.GetBlock().GetId() == predID {
		if err := c.compileSideEffectValue(sideEffect); err != nil {
			return llvm.Value{}, err
		}
		if resolved, ok := c.getCachedValue(sideEffect, edgeValID); ok && !resolved.IsNil() {
			return resolved, nil
		}
		return zero, nil
	}
	resolved, err := c.getValue(contextInst, edgeValID)
	if err != nil {
		return llvm.Value{}, err
	}
	if c.isSSAValueStored(edgeValID) {
		if predBB, ok := c.Blocks[predID]; ok && !predBB.IsNil() {
			c.setInsertPointBeforeTerminator(predBB)
			if c.function != nil {
				c.function.activeBlockID = predID
			}
			return c.loadSSAValue(edgeValID), nil
		}
	}
	return resolved, nil
}

func predecessorBlockIDs(fn *ssa.Function, blockID int64) []int64 {
	if fn == nil || blockID <= 0 {
		return nil
	}
	var preds []int64
	for _, fromID := range collectFunctionBlockIDs(fn) {
		fromVal, ok := fn.GetValueById(fromID)
		if !ok {
			continue
		}
		fromBB, ok := ssa.ToBasicBlock(fromVal)
		if !ok || fromBB == nil {
			continue
		}
		for _, succID := range fromBB.Succs {
			if succID == blockID {
				preds = append(preds, fromID)
				break
			}
		}
	}
	return preds
}

func (c *Compiler) gatherLLVMPredecessors(target llvm.BasicBlock) []llvm.BasicBlock {
	if target.IsNil() {
		return nil
	}
	targetID := c.blockIDForLLVM(target)
	if targetID <= 0 {
		return nil
	}

	var preds []llvm.BasicBlock
	for fromID, fromBB := range c.Blocks {
		if fromID == targetID {
			continue
		}
		if c.terminatorJumpsTo(fromBB, target) {
			preds = append(preds, fromBB)
		}
	}
	return preds
}

func (c *Compiler) blockIDForLLVM(target llvm.BasicBlock) int64 {
	for id, bb := range c.Blocks {
		if bb.C == target.C {
			return id
		}
	}
	return 0
}

func (c *Compiler) terminatorJumpsTo(from, target llvm.BasicBlock) bool {
	term := c.lastInstruction(from)
	if term.IsNil() || target.IsNil() {
		return false
	}
	targetPtr := unsafe.Pointer(target.C)
	matches := func(op llvm.Value) bool {
		if op.IsNil() {
			return false
		}
		return unsafe.Pointer(op.C) == targetPtr
	}
	switch term.NumOperands() {
	case 1:
		return matches(term.Operand(0))
	case 3:
		return matches(term.Operand(1)) || matches(term.Operand(2))
	default:
		return false
	}
}

func (c *Compiler) lastInstruction(bb llvm.BasicBlock) llvm.Value {
	var last llvm.Value
	for inst := bb.FirstInstruction(); !inst.IsNil(); inst = inst.NextInstruction() {
		last = inst
	}
	return last
}
