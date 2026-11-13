package ssa2source

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

type loopInfo struct {
	header  int64
	body    map[int64]bool
	latches []int64
	exit    int64
}

type structurer struct {
	function    *ssa.Function
	blocks      map[int64]*ssa.BasicBlock
	order       []int64
	succs       map[int64][]int64
	preds       map[int64][]int64
	dom         map[int64]map[int64]bool
	pdom        map[int64]map[int64]bool
	ipdom       map[int64]int64
	loops       map[int64]*loopInfo
	loopHeaders map[int64]bool
	emitted     map[int64]bool
	rendered    map[int64]int
	text        *strings.Builder
}

func newStructurer(function *ssa.Function) *structurer {
	s := &structurer{
		function:    function,
		blocks:      make(map[int64]*ssa.BasicBlock),
		succs:       make(map[int64][]int64),
		preds:       make(map[int64][]int64),
		dom:         make(map[int64]map[int64]bool),
		pdom:        make(map[int64]map[int64]bool),
		ipdom:       make(map[int64]int64),
		loops:       make(map[int64]*loopInfo),
		loopHeaders: make(map[int64]bool),
		emitted:     make(map[int64]bool),
		rendered:    make(map[int64]int),
	}
	for _, id := range function.Blocks {
		block, ok := function.GetBasicBlockByID(id)
		if !ok || block == nil {
			continue
		}
		s.blocks[id] = block
		s.order = append(s.order, id)
	}
	for id, block := range s.blocks {
		s.succs[id] = blockSuccessors(block)
	}
	for id, targets := range s.succs {
		for _, target := range targets {
			s.preds[target] = append(s.preds[target], id)
		}
	}
	s.computeDominators()
	s.computePostDominators()
	s.findLoops()
	return s
}

func blockSuccessors(block *ssa.BasicBlock) []int64 {
	if block == nil {
		return nil
	}
	if len(block.Succs) > 0 {
		return append([]int64(nil), block.Succs...)
	}
	if len(block.Insts) == 0 {
		return nil
	}
	instruction, ok := block.GetInstructionById(block.Insts[len(block.Insts)-1])
	if !ok || instruction == nil {
		return nil
	}
	switch value := instruction.(type) {
	case *ssa.If:
		return []int64{value.True, value.False}
	case *ssa.Jump:
		return []int64{value.To}
	default:
		return nil
	}
}

func (s *structurer) computeDominators() {
	entry := int64(0)
	if len(s.order) > 0 {
		entry = s.order[0]
	}
	for id := range s.blocks {
		s.dom[id] = make(map[int64]bool)
		for other := range s.blocks {
			s.dom[id][other] = true
		}
	}
	if entry != 0 {
		s.dom[entry] = map[int64]bool{entry: true}
	}
	for changed := true; changed; {
		changed = false
		for _, id := range s.order {
			if id == entry {
				continue
			}
			next := map[int64]bool{id: true}
			first := true
			for _, pred := range s.preds[id] {
				if len(s.dom[pred]) == 0 {
					continue
				}
				if first {
					for candidate := range s.dom[pred] {
						next[candidate] = true
					}
					first = false
					continue
				}
				for candidate := range next {
					if candidate != id && !s.dom[pred][candidate] {
						delete(next, candidate)
					}
				}
			}
			if first {
				continue
			}
			if !sameSet(next, s.dom[id]) {
				s.dom[id] = next
				changed = true
			}
		}
	}
}

func (s *structurer) computePostDominators() {
	const virtualExit = int64(0)
	for id := range s.blocks {
		s.pdom[id] = make(map[int64]bool)
		for other := range s.blocks {
			s.pdom[id][other] = true
		}
		s.pdom[id][virtualExit] = true
	}
	s.pdom[virtualExit] = map[int64]bool{virtualExit: true}
	for changed := true; changed; {
		changed = false
		for _, id := range s.order {
			next := map[int64]bool{id: true}
			first := true
			for _, succ := range s.succs[id] {
				if len(s.pdom[succ]) == 0 {
					continue
				}
				if first {
					for candidate := range s.pdom[succ] {
						next[candidate] = true
					}
					first = false
					continue
				}
				for candidate := range next {
					if candidate != id && !s.pdom[succ][candidate] {
						delete(next, candidate)
					}
				}
			}
			if first {
				next[virtualExit] = true
			}
			if !sameSet(next, s.pdom[id]) {
				s.pdom[id] = next
				changed = true
			}
		}
	}
	for _, id := range s.order {
		best := int64(0)
		bestSize := -1
		for candidate := range s.pdom[id] {
			if candidate == id {
				continue
			}
			size := len(s.pdom[candidate])
			if size > bestSize {
				best = candidate
				bestSize = size
			}
		}
		s.ipdom[id] = best
	}
}

func sameSet(left, right map[int64]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func (s *structurer) dominates(header, node int64) bool {
	return s.dom[node][header]
}

func (s *structurer) findLoops() {
	for _, tail := range s.order {
		for _, header := range s.succs[tail] {
			if header == tail || !s.dominates(header, tail) {
				continue
			}
			loop := s.loops[header]
			if loop == nil {
				loop = &loopInfo{header: header, body: map[int64]bool{header: true}}
				s.loops[header] = loop
				s.loopHeaders[header] = true
			}
			loop.latches = append(loop.latches, tail)
			s.collectLoopBody(loop, tail)
		}
	}
	for _, loop := range s.loops {
		for node := range loop.body {
			for _, succ := range s.succs[node] {
				if !loop.body[succ] {
					loop.exit = succ
					break
				}
			}
			if loop.exit != 0 {
				break
			}
		}
	}
}

func (s *structurer) collectLoopBody(loop *loopInfo, tail int64) {
	stack := []int64{tail}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if loop.body[node] {
			continue
		}
		loop.body[node] = true
		for _, pred := range s.preds[node] {
			if !loop.body[pred] {
				stack = append(stack, pred)
			}
		}
	}
}

func (s *structurer) render(text *strings.Builder) {
	if len(s.order) == 0 {
		return
	}
	s.text = text
	s.emitRegion(s.order[0], "    ", map[int64]bool{})
}

func (s *structurer) emitRegion(id int64, indent string, stop map[int64]bool) {
	s.emitRegionInternal(id, indent, stop, false)
}

func (s *structurer) emitRegionInternal(id int64, indent string, stop map[int64]bool, force bool) {
	if id == 0 || stop[id] {
		return
	}
	if s.emitted[id] && (!force || s.rendered[id] >= 2) {
		return
	}
	block := s.blocks[id]
	if block == nil {
		return
	}
	if loop := s.loops[id]; loop != nil {
		if !s.emitted[id] {
			s.emitLoop(loop, indent, stop)
		}
		return
	}
	s.emitted[id] = true
	s.rendered[id]++
	if ifInstruction := blockIf(block); ifInstruction != nil {
		s.renderBlockInstructions(block, indent, ifInstruction.Cond)
		s.emitIf(block, ifInstruction, indent, stop)
		return
	}
	s.renderBlockInstructions(block, indent, 0)
	if jump := blockJump(block); jump != nil {
		s.emitRegion(jump.To, indent, stop)
		return
	}
	for _, succ := range s.succs[id] {
		s.emitRegion(succ, indent, stop)
		return
	}
}

func (s *structurer) emitIf(block *ssa.BasicBlock, instruction *ssa.If, indent string, stop map[int64]bool) {
	condition := conditionExpression(instruction, instruction.Cond)
	join := s.ipdom[block.GetId()]
	if join != 0 && join != block.GetId() && (stop[join] || s.emitted[join] || !s.reachesJoin(instruction.True, join, stop) || !s.reachesJoin(instruction.False, join, stop)) {
		join = 0
	}
	if join == 0 {
		fmt.Fprintf(s.text, "%sif (%s) {\n", indent, condition)
		s.emitBranch(instruction.True, indent+"  ", stop)
		fmt.Fprintf(s.text, "%s} else {\n", indent)
		s.emitBranch(instruction.False, indent+"  ", stop)
		fmt.Fprintf(s.text, "%s}\n", indent)
		return
	}
	branchStop := copyStop(stop)
	branchStop[join] = true
	switch {
	case instruction.True == join:
		fmt.Fprintf(s.text, "%sif (!(%s)) {\n", indent, condition)
		s.emitBranch(instruction.False, indent+"  ", branchStop)
		fmt.Fprintf(s.text, "%s}\n", indent)
	case instruction.False == join:
		fmt.Fprintf(s.text, "%sif (%s) {\n", indent, condition)
		s.emitBranch(instruction.True, indent+"  ", branchStop)
		fmt.Fprintf(s.text, "%s}\n", indent)
	default:
		fmt.Fprintf(s.text, "%sif (%s) {\n", indent, condition)
		s.emitBranch(instruction.True, indent+"  ", branchStop)
		fmt.Fprintf(s.text, "%s} else {\n", indent)
		s.emitBranch(instruction.False, indent+"  ", branchStop)
		fmt.Fprintf(s.text, "%s}\n", indent)
	}
	s.emitRegion(join, indent, stop)
}

func (s *structurer) emitBranch(id int64, indent string, stop map[int64]bool) {
	if id == 0 || stop[id] {
		return
	}
	s.emitRegionInternal(id, indent, stop, s.emitted[id])
}

func (s *structurer) reachesJoin(start, join int64, stop map[int64]bool) bool {
	if start == 0 {
		return false
	}
	visited := map[int64]bool{}
	stack := []int64{start}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if node == join {
			return true
		}
		if node == 0 || visited[node] || stop[node] || s.loopHeaders[node] {
			continue
		}
		visited[node] = true
		block := s.blocks[node]
		if block == nil || blockReturn(block) || blockPanic(block) {
			continue
		}
		for _, succ := range s.succs[node] {
			stack = append(stack, succ)
		}
	}
	return false
}

func blockReturn(block *ssa.BasicBlock) bool {
	if block == nil || len(block.Insts) == 0 {
		return false
	}
	instruction, ok := block.GetInstructionById(block.Insts[len(block.Insts)-1])
	if !ok || instruction == nil {
		return false
	}
	_, ok = instruction.(*ssa.Return)
	return ok
}

func blockPanic(block *ssa.BasicBlock) bool {
	if block == nil || len(block.Insts) == 0 {
		return false
	}
	instruction, ok := block.GetInstructionById(block.Insts[len(block.Insts)-1])
	if !ok || instruction == nil {
		return false
	}
	_, ok = instruction.(*ssa.Panic)
	return ok
}

func (s *structurer) emitLoop(loop *loopInfo, indent string, stop map[int64]bool) {
	header := s.blocks[loop.header]
	if header == nil {
		return
	}
	s.emitted[loop.header] = true
	ifInstruction := blockIf(header)
	condition := "true"
	bodyStart := int64(0)
	exit := loop.exit
	if ifInstruction != nil {
		condition = conditionExpression(ifInstruction, ifInstruction.Cond)
		switch {
		case loop.body[ifInstruction.True] && !loop.body[ifInstruction.False]:
			bodyStart = ifInstruction.True
			exit = ifInstruction.False
		case loop.body[ifInstruction.False] && !loop.body[ifInstruction.True]:
			condition = "!(" + condition + ")"
			bodyStart = ifInstruction.False
			exit = ifInstruction.True
		default:
			bodyStart = ifInstruction.True
		}
	} else {
		for _, succ := range s.succs[loop.header] {
			if loop.body[succ] {
				bodyStart = succ
				break
			}
		}
	}
	conditionID := int64(0)
	if ifInstruction != nil {
		conditionID = ifInstruction.Cond
	}
	s.emitLoopPhis(loop, header, indent, false)
	s.renderBlockInstructions(header, indent, conditionID)
	if bodyStart == 0 {
		fmt.Fprintf(s.text, "%swhile (true) {\n", indent)
	} else {
		fmt.Fprintf(s.text, "%swhile (%s) {\n", indent, condition)
	}
	if bodyStart != 0 {
		bodyStop := copyStop(stop)
		bodyStop[loop.header] = true
		if exit != 0 {
			bodyStop[exit] = true
		}
		s.emitRegion(bodyStart, indent+"  ", bodyStop)
	}
	s.emitLoopPhis(loop, header, indent+"  ", true)
	fmt.Fprintf(s.text, "%s}\n", indent)
	if exit != 0 {
		s.emitRegion(exit, indent, stop)
	}
}

func (s *structurer) emitLoopPhis(loop *loopInfo, header *ssa.BasicBlock, indent string, backEdge bool) {
	phis := make([]*ssa.Phi, 0, len(header.Phis))
	for _, phiID := range header.Phis {
		instruction, ok := s.function.GetInstructionById(phiID)
		if !ok || instruction == nil {
			continue
		}
		phi, ok := ssa.ToPhi(instruction)
		if !ok || phi == nil || !valueUsed(phi) {
			continue
		}
		phis = append(phis, phi)
	}
	// Emit loop-carried values before one-edge aliases that may mention them.
	sort.SliceStable(phis, func(i, j int) bool {
		return len(phis[i].GetValues()) > len(phis[j].GetValues())
	})
	for _, phi := range phis {
		values := phi.GetValues()
		if len(values) == 0 {
			continue
		}
		if len(values) == 1 {
			if !backEdge {
				fmt.Fprintf(s.text, "%s%s = %s;\n", indent, assignName(phi), valueName(values[0]))
			}
			continue
		}
		value := values[0]
		if backEdge {
			value = values[len(values)-1]
		}
		fmt.Fprintf(s.text, "%s%s = %s;\n", indent, assignName(phi), valueName(value))
	}
}

func (s *structurer) renderBlockInstructions(block *ssa.BasicBlock, indent string, skipCondition int64) {
	for _, instructionID := range block.Insts {
		instruction, ok := s.function.GetInstructionById(instructionID)
		if !ok || instruction == nil {
			continue
		}
		switch instruction.(type) {
		case *ssa.Jump, *ssa.If:
			continue
		}
		if skipCondition != 0 && instruction.GetId() == skipCondition {
			continue
		}
		renderInstructionAt(s.text, instruction, indent)
	}
}

func blockIf(block *ssa.BasicBlock) *ssa.If {
	if block == nil || len(block.Insts) == 0 {
		return nil
	}
	instruction, ok := block.GetInstructionById(block.Insts[len(block.Insts)-1])
	if !ok || instruction == nil {
		return nil
	}
	value, _ := instruction.(*ssa.If)
	return value
}

func blockJump(block *ssa.BasicBlock) *ssa.Jump {
	if block == nil || len(block.Insts) == 0 {
		return nil
	}
	instruction, ok := block.GetInstructionById(block.Insts[len(block.Insts)-1])
	if !ok || instruction == nil {
		return nil
	}
	value, _ := instruction.(*ssa.Jump)
	return value
}

func copyStop(stop map[int64]bool) map[int64]bool {
	result := make(map[int64]bool, len(stop)+1)
	for id := range stop {
		result[id] = true
	}
	return result
}
