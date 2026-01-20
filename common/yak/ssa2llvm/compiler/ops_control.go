package compiler

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
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
	condVal, ok := c.Values[inst.Cond]
	if !ok {
		return fmt.Errorf("compileIf: condition value %d not found", inst.Cond)
	}

	// Check type of condition. If it's not i1, compare it with 0 to get i1.
	// We assume values are i64 for now as per Phase 1/2 assumption.
	if condVal.Type().IntTypeWidth() != 1 {
		// Create ICmp NE (Not Equal) 0
		zero := llvm.ConstInt(condVal.Type(), 0, false)
		condVal = c.Builder.CreateICmp(llvm.IntPredicate(llvm.IntNE), condVal, zero, "if_cond")
	}

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

// compilePhi creates the PHI node but DOES NOT populate incoming values.
// This is Pass 1 of Phi handling.
func (c *Compiler) compilePhi(inst *ssa.Phi) error {
	// Assume i64 type for now
	phiNode := c.Builder.CreatePHI(c.LLVMCtx.Int64Type(), fmt.Sprintf("phi_%d", inst.GetId()))
	c.Values[inst.GetId()] = phiNode
	return nil
}

// resolvePhi populates the incoming values for a PHI node.
// This is Pass 2 of Phi handling, called after all blocks are generated.
func (c *Compiler) resolvePhi(inst *ssa.Phi) error {
	phiVal, ok := c.Values[inst.GetId()]
	if !ok {
		return fmt.Errorf("resolvePhi: phi value %d not found", inst.GetId())
	}
	// Removed IsAPHI check and IsNil check to avoid binding issues.

	// Find the block where this Phi resides.
	// inst.CFGEntryBasicBlock gives us the Block ID.
	blockID := inst.CFGEntryBasicBlock

	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("resolvePhi: function for phi %d is nil", inst.GetId())
	}

	blockVal, ok := fn.GetValueById(blockID)
	if !ok {
		return fmt.Errorf("resolvePhi: block %d not found in function", blockID)
	}

	// Check if blockVal is actually a BasicBlock
	bbSsa, ok := blockVal.(*ssa.BasicBlock)
	if !ok {
		// Attempt cast via interface if needed or check if it's *ssa.BasicBlock
		return fmt.Errorf("resolvePhi: value %d is not *ssa.BasicBlock", blockID)
	}

	// Verify consistency between Edge (Values) and Preds (Blocks)
	// ssa.Phi struct has Edge []int64 (Values)
	// ssa.BasicBlock struct has Preds []int64 (BasicBlocks)
	// They should match in length and order.

	// We assume inst.Edge corresponds to bbSsa.Preds by index.

	edges := inst.Edge
	preds := bbSsa.Preds

	if len(edges) != len(preds) {
		return fmt.Errorf("resolvePhi: mismatch edges count (%d) and preds count (%d) for phi %d", len(edges), len(preds), inst.GetId())
	}

	var incomingVals []llvm.Value
	var incomingBlocks []llvm.BasicBlock

	for i, edgeValID := range edges {
		// Value
		val, ok := c.Values[edgeValID]
		if !ok {
			return fmt.Errorf("resolvePhi: incoming value %d not found", edgeValID)
		}

		// Block
		predBlockID := preds[i]
		blk, ok := c.Blocks[predBlockID]
		if !ok {
			return fmt.Errorf("resolvePhi: incoming block %d not found", predBlockID)
		}

		incomingVals = append(incomingVals, val)
		incomingBlocks = append(incomingBlocks, blk)
	}

	phiVal.AddIncoming(incomingVals, incomingBlocks)
	return nil
}
