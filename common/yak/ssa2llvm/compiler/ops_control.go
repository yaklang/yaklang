package compiler

import (
	"fmt"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// compileJump creates an unconditional branch to the target block.
func (c *Compiler) compileJump(inst *ssa.Jump) error {
	targetBlock, ok := c.Blocks[inst.To]
	if !ok {
		return fmt.Errorf("compileJump: target block %d not found", inst.To)
	}
	c.Builder.CreateBr(targetBlock)
	return nil
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

// compilePhi creates the PHI node but DOES NOT populate incoming values.
// This is Pass 1 of Phi handling.
func (c *Compiler) compilePhi(inst *ssa.Phi) error {
	phiType := c.inferPhiType(inst)
	phiNode := c.Builder.CreatePHI(phiType, fmt.Sprintf("phi_%d", inst.GetId()))
	c.Values[inst.GetId()] = phiNode
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
		case ssa.ObjectTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind, ssa.PointerKind, ssa.StringTypeKind:
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

// resolvePhi populates the incoming values for a PHI node.
// This is Pass 2 of Phi handling, called after all blocks are generated.
func (c *Compiler) resolvePhi(inst *ssa.Phi) error {
	phiVal, ok := c.Values[inst.GetId()]
	if !ok {
		return fmt.Errorf("resolvePhi: phi value %d not found", inst.GetId())
	}

	block := inst.GetBlock()
	if block == nil {
		return fmt.Errorf("resolvePhi: phi %d has no block", inst.GetId())
	}
	blockID := block.GetId()

	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("resolvePhi: function for phi %d is nil", inst.GetId())
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

	llvmPreds := make([]llvm.BasicBlock, 0, len(preds))
	for _, predBlockID := range preds {
		predBB, ok := c.Blocks[predBlockID]
		if !ok || predBB.IsNil() {
			continue
		}
		if !c.terminatorJumpsTo(predBB, phiBB) {
			continue
		}
		llvmPreds = append(llvmPreds, predBB)
	}
	if len(llvmPreds) == 0 {
		for _, predBlockID := range preds {
			if blk, ok := c.Blocks[predBlockID]; ok && !blk.IsNil() {
				llvmPreds = append(llvmPreds, blk)
			}
		}
	}

	zero := llvm.ConstInt(c.inferPhiType(inst), 0, false)
	incomingVals := make([]llvm.Value, 0, len(llvmPreds))
	incomingBlocks := make([]llvm.BasicBlock, 0, len(llvmPreds))

	prevActive := int64(0)
	if c.function != nil {
		prevActive = c.function.activeBlockID
	}
	defer func() {
		if c.function != nil {
			c.function.activeBlockID = prevActive
		}
	}()

	for _, predBB := range llvmPreds {
		val := zero
		predID := c.blockIDForLLVM(predBB)
		if predID > 0 {
			c.setInsertPointBeforeTerminator(predBB)
			if c.function != nil {
				c.function.activeBlockID = predID
			}
			if edgeValID, ok := edgeByPred[predID]; ok {
				if edgeObj, ok := fn.GetValueById(edgeValID); ok && edgeObj != nil {
					if _, isUndef := edgeObj.(*ssa.Undefined); !isUndef {
						if resolved, err := c.resolvePhiIncomingValue(inst, fn, predID, edgeValID); err == nil {
							val = resolved
						}
					}
				}
			}
		}
		incomingVals = append(incomingVals, val)
		incomingBlocks = append(incomingBlocks, predBB)
	}

	if len(incomingVals) > 0 {
		phiVal.AddIncoming(incomingVals, incomingBlocks)
	}

	return nil
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
	if edgePhi, ok := edgeObj.(*ssa.Phi); ok && edgePhi != nil {
		phiBlockID := int64(0)
		if edgePhi.GetBlock() != nil {
			phiBlockID = edgePhi.GetBlock().GetId()
		}
		if phiBlockID != predID {
			return zero, nil
		}
	}
	return c.getValue(contextInst, edgeValID)
}

func predecessorBlockIDs(fn *ssa.Function, blockID int64) []int64 {
	if fn == nil || blockID <= 0 {
		return nil
	}
	var preds []int64
	for _, fromID := range fn.Blocks {
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
