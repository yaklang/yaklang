package lowir2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// planMergePhis creates a phi at every join where a register carries a value from
// more than one path. Without it a register read after a join returns whichever
// write came last in lifting order, which has no SSA meaning: the two arms usually
// assign different values.
//
// The join points come from the iterated dominance frontier of the blocks that
// write the register, which is the standard construction.
func (c *converter) planMergePhis(function core.Function) {
	c.mergePhis = make(map[core.BlockID]map[string]*ssa.Phi)
	if c.blockOut == nil {
		// Keep what bindParameters recorded: an entry value is the edge value a
		// first block supplies.
		c.blockOut = make(map[core.BlockID]map[string]ssa.Value)
	}
	known := make(map[core.BlockID]bool, len(function.Blocks))
	for _, block := range function.Blocks {
		known[block.ID] = true
	}
	preds := make(map[core.BlockID][]core.BlockID, len(function.Blocks))
	succs := make(map[core.BlockID][]core.BlockID, len(function.Blocks))
	for _, block := range function.Blocks {
		for _, target := range blockSuccessors(block) {
			if !known[target] {
				continue
			}
			succs[block.ID] = appendUniqueBlock(succs[block.ID], target)
			preds[target] = appendUniqueBlock(preds[target], block.ID)
		}
	}
	c.predsOf = preds
	entry := function.Blocks[0].ID
	c.idom = dominators(entry, preds, succs)
	frontier := dominanceFrontier(preds, c.idom)
	defs := make(map[string][]core.BlockID)
	// order keeps the registers in the order the function first writes them, which
	// fixes phi identifiers without sorting on every walk.
	var order []string
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
				continue
			}
			operand := instruction.Operands[0]
			view := c.registerView(operand.Name, operand.Type.Bits)
			if view.canonical == "" || c.isFlagsRegister(view.canonical) {
				// A comparison result is bound to its own flag value, not merged as
				// data: the condition read looks the comparison up by value id.
				continue
			}
			key := "reg:" + view.canonical
			if _, seen := defs[key]; !seen {
				order = append(order, key)
			}
			defs[key] = appendUniqueBlock(defs[key], block.ID)
		}
	}
	// phi identifiers and the printed text follow that order, so nothing here
	// depends on map iteration.
	for _, key := range order {
		c.insertMergePhi(key, defs[key], frontier)
	}
}

// insertMergePhi walks the dominance frontier of the defining blocks and creates
// one phi per join, following the classic construction.
func (c *converter) insertMergePhi(key string, defs []core.BlockID, frontier map[core.BlockID][]core.BlockID) {
	defined := make(map[core.BlockID]bool, len(defs))
	for _, id := range defs {
		defined[id] = true
	}
	inserted := make(map[core.BlockID]bool)
	work := append([]core.BlockID(nil), defs...)
	for len(work) > 0 {
		blockID := work[0]
		work = work[1:]
		for _, join := range frontier[blockID] {
			if inserted[join] || c.blocks[join] == nil {
				continue
			}
			inserted[join] = true
			if c.newMergePhi(join, key) != nil && !defined[join] {
				work = append(work, join)
			}
		}
	}
}

func (c *converter) newMergePhi(blockID core.BlockID, key string) *ssa.Phi {
	target := c.blocks[blockID]
	if target == nil {
		return nil
	}
	phi := ssa.NewPhi(target, key)
	if phi == nil {
		return nil
	}
	target.Phis = append(target.Phis, phi.GetId())
	if c.mergePhis[blockID] == nil {
		c.mergePhis[blockID] = make(map[string]*ssa.Phi)
	}
	if _, seen := c.mergePhis[blockID][key]; !seen {
		c.mergePhiOrder[blockID] = append(c.mergePhiOrder[blockID], key)
	}
	c.mergePhis[blockID][key] = phi
	return phi
}

// mergePhi returns the phi the current block uses for one register.
func (c *converter) mergePhi(name string) *ssa.Phi {
	if c.isFlagsRegister(name) {
		return nil
	}
	phis := c.mergePhis[c.current]
	if phis == nil {
		return nil
	}
	return phis["reg:"+name]
}

// isFlagsRegister reports whether a register carries a comparison result. Such a
// register is not merged: the condition read resolves the comparison by the value
// id of the flag write.
func (c *converter) isFlagsRegister(name string) bool {
	if name == "" {
		return false
	}
	flags := c.facts.Registers.Flags
	if flags == "" {
		return false
	}
	return strings.EqualFold(name, c.registerView(flags, 0).canonical)
}

// recordRegisterValue remembers the value a block leaves in a register, which is
// the operand of the edge into a successor.
func (c *converter) recordRegisterValue(name string, value ssa.Value) {
	if value == nil {
		return
	}
	key := "reg:" + name
	if c.blockOut[c.current] == nil {
		c.blockOut[c.current] = make(map[string]ssa.Value)
	}
	c.blockOut[c.current][key] = value
}

// fillMergePhis fills the edge operands once every block is converted, because a
// phi edge follows the predecessor order the builder recorded.
func (c *converter) fillMergePhis() {
	// Blocks follow program order and phis follow creation order. Filling an edge
	// can create a value for a predecessor that leaves nothing behind, and the
	// order those values are created in decides their identifiers and the printed
	// text, so neither walk may depend on map iteration.
	for _, blockID := range c.blockOrder {
		if len(c.mergePhiOrder[blockID]) == 0 {
			continue
		}
		block := c.blocks[blockID]
		if block == nil || len(block.Preds) == 0 {
			continue
		}
		for _, key := range c.mergePhiOrder[blockID] {
			phi := c.mergePhis[blockID][key]
			edge := make([]int64, 0, len(block.Preds))
			var edgeType ssa.Type
			for _, pred := range block.Preds {
				value := c.outgoingRegisterValue(c.lowirBySSA[pred], key)
				if value == nil {
					value = c.undefinedValue(key)
				}
				if value == nil {
					break
				}
				if edgeType == nil {
					edgeType = value.GetType()
				}
				edge = append(edge, value.GetId())
				value.AddUser(phi)
			}
			if len(edge) != len(block.Preds) {
				continue
			}
			phi.Edge = edge
			if edgeType != nil {
				phi.SetType(edgeType)
			}
		}
	}
}

// outgoingRegisterValue returns the value a predecessor leaves in a register. A
// block that does not write the register keeps the value it received from its own
// predecessors, so the search follows the dominator chain until it finds a write
// or a phi.
func (c *converter) outgoingRegisterValue(blockID core.BlockID, key string) ssa.Value {
	if blockID == 0 {
		return nil
	}
	node := blockID
	for {
		if values := c.blockOut[node]; values != nil {
			if value := values[key]; value != nil {
				return value
			}
		}
		if phis := c.mergePhis[node]; phis != nil {
			if phi := phis[key]; phi != nil {
				return phi
			}
		}
		if phis := c.loopPhis[node]; phis != nil {
			if phi := phis[key]; phi != nil {
				return phi
			}
		}
		parent, ok := c.idom[node]
		if !ok || parent == node || parent == 0 {
			return nil
		}
		node = parent
	}
}

func appendUniqueBlock(list []core.BlockID, value core.BlockID) []core.BlockID {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

// dominators computes the immediate dominator of every reachable block with the
// iterative algorithm, which only needs a reverse postorder.
func dominators(entry core.BlockID, preds, succs map[core.BlockID][]core.BlockID) map[core.BlockID]core.BlockID {
	order := reversePostorder(entry, succs)
	position := make(map[core.BlockID]int, len(order))
	for index, id := range order {
		position[id] = index
	}
	idom := map[core.BlockID]core.BlockID{entry: entry}
	intersect := func(left, right core.BlockID) core.BlockID {
		for left != right {
			for position[left] > position[right] {
				next, ok := idom[left]
				if !ok || next == left {
					return entry
				}
				left = next
			}
			for position[right] > position[left] {
				next, ok := idom[right]
				if !ok || next == right {
					return entry
				}
				right = next
			}
		}
		return left
	}
	changed := true
	for changed {
		changed = false
		for _, id := range order {
			if id == entry {
				continue
			}
			var candidate core.BlockID
			for _, pred := range preds[id] {
				if _, ok := position[pred]; !ok {
					continue
				}
				if _, known := idom[pred]; !known {
					continue
				}
				if candidate == 0 {
					candidate = pred
					continue
				}
				candidate = intersect(pred, candidate)
			}
			if candidate == 0 {
				continue
			}
			if current, ok := idom[id]; !ok || current != candidate {
				idom[id] = candidate
				changed = true
			}
		}
	}
	return idom
}

func reversePostorder(entry core.BlockID, succs map[core.BlockID][]core.BlockID) []core.BlockID {
	var order []core.BlockID
	visited := map[core.BlockID]bool{}
	var visit func(core.BlockID)
	visit = func(id core.BlockID) {
		if visited[id] {
			return
		}
		visited[id] = true
		for _, next := range succs[id] {
			visit(next)
		}
		order = append(order, id)
	}
	visit(entry)
	for left, right := 0, len(order)-1; left < right; left, right = left+1, right-1 {
		order[left], order[right] = order[right], order[left]
	}
	return order
}

// dominanceFrontier returns, for every block, the join points its definitions
// reach.
func dominanceFrontier(preds map[core.BlockID][]core.BlockID, idom map[core.BlockID]core.BlockID) map[core.BlockID][]core.BlockID {
	frontier := make(map[core.BlockID][]core.BlockID)
	for id, blockPreds := range preds {
		if len(blockPreds) < 2 {
			continue
		}
		parent, ok := idom[id]
		if !ok {
			continue
		}
		for _, pred := range blockPreds {
			if _, reachable := idom[pred]; !reachable {
				continue
			}
			runner := pred
			for runner != parent {
				frontier[runner] = appendUniqueBlock(frontier[runner], id)
				next, ok := idom[runner]
				if !ok || next == runner {
					break
				}
				runner = next
			}
		}
	}
	return frontier
}

// blockSuccessors returns the blocks one block can continue to.
func blockSuccessors(block core.Block) []core.BlockID {
	if len(block.Instructions) == 0 {
		return nil
	}
	last := block.Instructions[len(block.Instructions)-1]
	if last.Kind != core.OpJump && last.Kind != core.OpBranch {
		return nil
	}
	return last.Targets
}
