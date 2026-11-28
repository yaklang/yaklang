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
	domIndex    map[int64]int
	domEntry    []int
	domExit     []int
	ipdom       map[int64]int64
	loops       map[int64]*loopInfo
	loopHeaders map[int64]bool
	loopExit    int64
	emitted     map[int64]bool
	rendered    map[int64]int
	// printedMergePhi marks a phi already written by the arm that supplies it.
	printedMergePhi map[int64]bool
	text            *strings.Builder
}

func newStructurer(function *ssa.Function) *structurer {
	s := &structurer{
		function:        function,
		blocks:          make(map[int64]*ssa.BasicBlock, len(function.Blocks)),
		order:           make([]int64, 0, len(function.Blocks)),
		succs:           make(map[int64][]int64, len(function.Blocks)),
		preds:           make(map[int64][]int64, len(function.Blocks)),
		ipdom:           make(map[int64]int64, len(function.Blocks)),
		loops:           make(map[int64]*loopInfo),
		loopHeaders:     make(map[int64]bool),
		printedMergePhi: make(map[int64]bool),
		emitted:         make(map[int64]bool, len(function.Blocks)),
		rendered:        make(map[int64]int, len(function.Blocks)),
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
	for _, id := range s.order {
		for _, target := range s.succs[id] {
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

// computeIterativeDominators runs Cooper-Harvey-Kennedy on nodes[0..n-1] in
// reverse postorder (where node 0 is the root), reading predecessors via preds(rpoIdx).
func computeIterativeDominators(n int, preds func(rpoIdx int) []int) []int {
	idom := make([]int, n)
	for i := range idom {
		idom[i] = -1
	}
	if n == 0 {
		return idom
	}
	idom[0] = 0
	intersect := func(b1, b2 int) int {
		f1, f2 := b1, b2
		for f1 != f2 {
			for f1 > f2 {
				f1 = idom[f1]
			}
			for f2 > f1 {
				f2 = idom[f2]
			}
		}
		return f1
	}
	for changed := true; changed; {
		changed = false
		for i := 1; i < n; i++ {
			newIdom := -1
			for _, p := range preds(i) {
				if p < 0 || idom[p] < 0 {
					continue
				}
				if newIdom < 0 {
					newIdom = p
				} else {
					newIdom = intersect(p, newIdom)
				}
			}
			if newIdom >= 0 && idom[i] != newIdom {
				idom[i] = newIdom
				changed = true
			}
		}
	}
	return idom
}

func reversePostorderIDs(entry int64, blocks map[int64]*ssa.BasicBlock, adj func(int64) []int64) []int64 {
	type frame struct {
		id  int64
		pos int
	}
	visited := make(map[int64]bool, len(blocks))
	post := make([]int64, 0, len(blocks))
	visited[entry] = true
	stack := []frame{{id: entry}}
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		nbrs := adj(top.id)
		if top.pos < len(nbrs) {
			next := nbrs[top.pos]
			top.pos++
			if !visited[next] {
				if _, ok := blocks[next]; ok {
					visited[next] = true
					stack = append(stack, frame{id: next})
				}
			}
			continue
		}
		post = append(post, top.id)
		stack = stack[:len(stack)-1]
	}
	for i, j := 0, len(post)-1; i < j; i, j = i+1, j-1 {
		post[i], post[j] = post[j], post[i]
	}
	return post
}

func (s *structurer) computeDominators() {
	if len(s.order) == 0 {
		return
	}
	rpo := reversePostorderIDs(s.order[0], s.blocks, func(id int64) []int64 { return s.succs[id] })
	n := len(rpo)
	s.domIndex = make(map[int64]int, n)
	for i, id := range rpo {
		s.domIndex[id] = i
	}
	idom := computeIterativeDominators(n, func(i int) []int {
		ps := s.preds[rpo[i]]
		out := make([]int, 0, len(ps))
		for _, p := range ps {
			if idx, ok := s.domIndex[p]; ok {
				out = append(out, idx)
			}
		}
		return out
	})
	children := make([][]int, n)
	for i := 1; i < n; i++ {
		if p := idom[i]; p >= 0 && p != i {
			children[p] = append(children[p], i)
		}
	}
	s.domEntry = make([]int, n)
	s.domExit = make([]int, n)
	type eulerFrame struct {
		node int
		pos  int
	}
	clock := 0
	stack := []eulerFrame{{node: 0}}
	clock++
	s.domEntry[0] = clock
	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.pos < len(children[top.node]) {
			child := children[top.node][top.pos]
			top.pos++
			clock++
			s.domEntry[child] = clock
			stack = append(stack, eulerFrame{node: child})
			continue
		}
		clock++
		s.domExit[top.node] = clock
		stack = stack[:len(stack)-1]
	}
}

func (s *structurer) computePostDominators() {
	if len(s.order) == 0 {
		return
	}
	const virtualExit = int64(0)
	revBlocks := make(map[int64]*ssa.BasicBlock, len(s.blocks)+1)
	for k, v := range s.blocks {
		revBlocks[k] = v
	}
	revBlocks[virtualExit] = nil
	var exitSuccs []int64
	for _, id := range s.order {
		hasMappedSucc := false
		for _, succ := range s.succs[id] {
			if _, ok := s.blocks[succ]; ok && succ != virtualExit {
				hasMappedSucc = true
				break
			}
		}
		if !hasMappedSucc {
			exitSuccs = append(exitSuccs, id)
		}
	}
	rpo := reversePostorderIDs(virtualExit, revBlocks, func(id int64) []int64 {
		if id == virtualExit {
			return exitSuccs
		}
		return s.preds[id]
	})
	n := len(rpo)
	rpoIdx := make(map[int64]int, n)
	for i, id := range rpo {
		rpoIdx[id] = i
	}
	ipdomIdx := computeIterativeDominators(n, func(i int) []int {
		id := rpo[i]
		succs := s.succs[id]
		out := make([]int, 0, len(succs)+1)
		hasMapped := false
		for _, succ := range succs {
			if idx, ok := rpoIdx[succ]; ok {
				out = append(out, idx)
				hasMapped = true
			}
		}
		if !hasMapped && id != virtualExit {
			out = append(out, 0)
		}
		return out
	})
	for i := 1; i < n; i++ {
		if p := ipdomIdx[i]; p > 0 {
			s.ipdom[rpo[i]] = rpo[p]
		} else {
			s.ipdom[rpo[i]] = 0
		}
	}
}

func (s *structurer) dominates(header, node int64) bool {
	hi, okH := s.domIndex[header]
	ni, okN := s.domIndex[node]
	if !okH || !okN {
		return false
	}
	return s.domEntry[hi] <= s.domEntry[ni] && s.domExit[ni] <= s.domExit[hi]
}

func (s *structurer) findLoops() {
	for _, tail := range s.order {
		for _, header := range s.succs[tail] {
			if !s.dominates(header, tail) {
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
	if len(s.loops) == 0 {
		return
	}
	position := make(map[int64]int, len(s.order))
	for index, node := range s.order {
		position[node] = index
	}
	for _, loop := range s.loops {
		first := len(s.order)
		for node := range loop.body {
			index, known := position[node]
			if !known || index >= first {
				continue
			}
			for _, succ := range s.succs[node] {
				if succ != 0 && !loop.body[succ] {
					first = index
					loop.exit = succ
					break
				}
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
		if jump.To == s.loopExit {
			fmt.Fprintf(s.text, "%sbreak;\n", indent)
			return
		}
		s.emitRegion(jump.To, indent, stop)
		return
	}
	for _, succ := range s.succs[id] {
		if succ == s.loopExit {
			fmt.Fprintf(s.text, "%sbreak;\n", indent)
			return
		}
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
		if s.armEndsAt(instruction.True, join) && s.armEndsAt(instruction.False, join) {
			// One merge value and nothing else in the arms: the diamond is the
			// conditional expression it came from.
			if s.armsAreEmpty(instruction.True, join) && s.armsAreEmpty(instruction.False, join) &&
				s.emitMergeTernary(join, instruction.True, instruction.False, condition, indent) {
				s.emitRegion(join, indent, stop)
				return
			}
			// Both arms merge directly. The values they supply are the phi edges, so
			// print them at the end of each arm and let the merge continue.
			fmt.Fprintf(s.text, "%sif (%s) {\n", indent, condition)
			s.emitBranch(instruction.True, indent+"  ", branchStop)
			s.printMergeEdges(s.blocks[join], instruction.True, indent+"  ")
			fmt.Fprintf(s.text, "%s} else {\n", indent)
			s.emitBranch(instruction.False, indent+"  ", branchStop)
			s.printMergeEdges(s.blocks[join], instruction.False, indent+"  ")
			fmt.Fprintf(s.text, "%s}\n", indent)
			s.emitRegion(join, indent, stop)
			return
		}
		fmt.Fprintf(s.text, "%sif (%s) {\n", indent, condition)
		s.emitBranch(instruction.True, indent+"  ", branchStop)
		fmt.Fprintf(s.text, "%s} else {\n", indent)
		s.emitBranch(instruction.False, indent+"  ", branchStop)
		fmt.Fprintf(s.text, "%s}\n", indent)
	}
	s.emitRegion(join, indent, stop)
}

func (s *structurer) emitBranch(id int64, indent string, stop map[int64]bool) {
	if id == 0 {
		return
	}
	// Inside a loop body a branch to the exit is a break. The exit block is
	// emitted once after the loop, so the arm stays a single statement.
	if id == s.loopExit {
		fmt.Fprintf(s.text, "%sbreak;\n", indent)
		return
	}
	if stop[id] {
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
			condition = negatedCondition(ifInstruction, ifInstruction.Cond)
			bodyStart = ifInstruction.False
			exit = ifInstruction.True
		case loop.body[ifInstruction.True] && loop.body[ifInstruction.False]:
			// Both arms stay inside the loop, so the header branch does not leave
			// it: it only selects which path computes the loop test. Clang -O0
			// emits that path as "exit when the test is true", so the continuation
			// condition is the negation and the false arm reaches the body. The
			// test itself is re-rendered at the back edge.
			condition = negatedCondition(ifInstruction, ifInstruction.Cond)
			bodyStart = ifInstruction.False
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
	if len(loop.body) == 1 && bodyStart == loop.header {
		s.emitLoopPhis(loop, header, indent, false)
		fmt.Fprintf(s.text, "%swhile (true) {\n", indent)
		s.renderBlockInstructions(header, indent+"  ", conditionID)
		if ifInstruction != nil && exit != 0 {
			fmt.Fprintf(s.text, "%s  if (!(%s)) {\n", indent, condition)
			fmt.Fprintf(s.text, "%s    break;\n", indent)
			fmt.Fprintf(s.text, "%s  }\n", indent)
		}
		s.emitLoopPhis(loop, header, indent+"  ", true)
		fmt.Fprintf(s.text, "%s}\n", indent)
		if exit != 0 {
			s.emitRegion(exit, indent, stop)
		}
		return
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
		// A branch to the exit from inside the body is a break. The exit block
		// itself is emitted after the loop, so the arm does not inline it.
		previousExit := s.loopExit
		s.loopExit = exit
		s.emitRegion(bodyStart, indent+"  ", bodyStop)
		s.loopExit = previousExit
	}
	s.emitLoopPhis(loop, header, indent+"  ", true)
	// The header's own instructions compute the next iteration's condition from the
	// values the back edge just wrote. They ran once before the loop; repeating
	// them here is what makes the while condition depend on the current index
	// instead of the value it had at entry.
	s.renderBlockInstructions(header, indent+"  ", conditionID)
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
				name := assignName(values[0])
				if _, isConst := ssa.ToConstInst(values[0]); isConst {
					// A constant has no assignment line of its own, so it is written here.
					name = valueName(values[0])
				}
				fmt.Fprintf(s.text, "%s%s = %s;\n", indent, assignName(phi), name)
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
	if s.loops[block.GetId()] == nil {
		s.renderMergePhis(block, indent)
	}
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

// renderMergePhis prints the phis of a join block. A loop header prints its own
// phis while the loop is emitted, so this skips it.
//
// A merge phi stands for one variable on every incoming edge, and the statements
// that follow read it as a single value, so the first edge names it.
func (s *structurer) renderMergePhis(block *ssa.BasicBlock, indent string) {
	for _, phiID := range block.Phis {
		if s.printedMergePhi[phiID] {
			continue
		}
		instruction, ok := s.function.GetInstructionById(phiID)
		if !ok || instruction == nil {
			continue
		}
		phi, ok := ssa.ToPhi(instruction)
		if !ok || phi == nil || !valueUsed(phi) {
			continue
		}
		values := phi.GetValues()
		if len(values) == 0 {
			continue
		}
		name := assignName(representativePhiValue(values))
		if _, isConst := ssa.ToConstInst(values[0]); isConst {
			// A constant has no assignment line of its own, so it is written here.
			name = valueName(values[0])
		}
		fmt.Fprintf(s.text, "%s%s = %s;\n", indent, assignName(phi), name)
	}
}

// representativePhiValue picks the value a join is printed with. An entry
// parameter is the last choice: a register nobody wrote before the branch only
// happens to hold it, and naming it would pull a parameter into the signature.
func representativePhiValue(values []ssa.Value) ssa.Value {
	for _, value := range values {
		if value == nil {
			continue
		}
		if _, isParameter := value.(*ssa.Parameter); isParameter {
			continue
		}
		return value
	}
	return values[0]
}

// singleJumpTarget reports whether a block holds one unconditional jump and
// nothing else.
func singleJumpTarget(block *ssa.BasicBlock) (int64, bool) {
	if block == nil || len(block.Insts) != 1 || len(block.Phis) != 0 {
		return 0, false
	}
	instruction, ok := block.GetInstructionById(block.Insts[0])
	if !ok || instruction == nil {
		return 0, false
	}
	jump, ok := instruction.(*ssa.Jump)
	if !ok || jump == nil {
		return 0, false
	}
	return jump.To, true
}

// emitMergeArms prints a diamond whose arms only carry the values a join phi
// selects. Each arm assigns its own edge, which is the conditional expression the
// original instruction stood for.
func (s *structurer) emitMergeArms(join, thenBlock, elseBlock int64, condition, indent string) {
	joinBlock := s.blocks[join]
	if joinBlock == nil {
		return
	}
	fmt.Fprintf(s.text, "%sif (%s) {\n", indent, condition)
	s.printMergeEdges(joinBlock, thenBlock, indent+"  ")
	fmt.Fprintf(s.text, "%s} else {\n", indent)
	s.printMergeEdges(joinBlock, elseBlock, indent+"  ")
	fmt.Fprintf(s.text, "%s}\n", indent)
}

func (s *structurer) printMergeEdges(joinBlock *ssa.BasicBlock, pred int64, indent string) {
	index := -1
	for at, candidate := range joinBlock.Preds {
		if candidate == pred {
			index = at
			break
		}
	}
	if index < 0 {
		return
	}
	for _, phiID := range joinBlock.Phis {
		instruction, ok := s.function.GetInstructionById(phiID)
		if !ok || instruction == nil {
			continue
		}
		phi, ok := ssa.ToPhi(instruction)
		if !ok || phi == nil || index >= len(phi.Edge) {
			continue
		}
		value, ok := phi.GetValueById(phi.Edge[index])
		if !ok || value == nil {
			continue
		}
		fmt.Fprintf(s.text, "%s%s = %s;\n", indent, assignName(phi), valueName(value))
		s.printedMergePhi[phi.GetId()] = true
	}
}

// emitMergeTernary prints a diamond that selects one value as a conditional
// expression. It reports false when the merge carries anything else, because then
// the branches do more than choose a value.
func (s *structurer) emitMergeTernary(join, thenBlock, elseBlock int64, condition, indent string) bool {
	joinBlock := s.blocks[join]
	if joinBlock == nil || len(joinBlock.Phis) != 1 {
		return false
	}
	phiID := joinBlock.Phis[0]
	instruction, ok := s.function.GetInstructionById(phiID)
	if !ok || instruction == nil {
		return false
	}
	phi, ok := ssa.ToPhi(instruction)
	if !ok || phi == nil || len(phi.Edge) != len(joinBlock.Preds) {
		return false
	}
	whenTrue, ok := mergeEdgeValue(phi, joinBlock, thenBlock)
	if !ok {
		return false
	}
	whenFalse, ok := mergeEdgeValue(phi, joinBlock, elseBlock)
	if !ok {
		return false
	}
	fmt.Fprintf(s.text, "%s%s = (%s ? %s : %s);\n", indent, assignName(phi), condition, valueName(whenTrue), valueName(whenFalse))
	s.printedMergePhi[phiID] = true
	return true
}

// mergeEdgeValue returns the value a phi takes on the edge from one predecessor.
func mergeEdgeValue(phi *ssa.Phi, joinBlock *ssa.BasicBlock, pred int64) (ssa.Value, bool) {
	for at, candidate := range joinBlock.Preds {
		if candidate != pred || at >= len(phi.Edge) {
			continue
		}
		value, ok := phi.GetValueById(phi.Edge[at])
		if !ok || value == nil {
			return nil, false
		}
		return value, true
	}
	return nil, false
}

// armEndsAt reports whether a block ends with a jump to the merge block. The arm
// may hold other instructions; only its exit matters.
func (s *structurer) armEndsAt(id, join int64) bool {
	block := s.blocks[id]
	if block == nil || len(block.Insts) == 0 {
		return false
	}
	instruction, ok := block.GetInstructionById(block.Insts[len(block.Insts)-1])
	if !ok || instruction == nil {
		return false
	}
	jump, ok := instruction.(*ssa.Jump)
	if !ok || jump == nil {
		return false
	}
	return jump.To == join
}

// armsAreEmpty reports whether a block does nothing but jump to the merge.
func (s *structurer) armsAreEmpty(id, join int64) bool {
	target, ok := singleJumpTarget(s.blocks[id])
	return ok && target == join
}
