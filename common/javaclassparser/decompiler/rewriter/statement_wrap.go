package rewriter

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// sortNodesByID orders nodes by their (unique, deterministic) statement id in place and returns the
// slice. Set.List() and maps.Keys/Values iterate a Go map, so their order varies run to run; feeding
// that order into CFG structuring (loop out-points, merge-node selection, rewrite ordering) makes the
// same class decompile to different output - and occasionally stub with "multiple next" - depending on
// map randomization. Sorting by node id restores determinism without changing which nodes are present.
func sortNodesByID(nodes []*core.Node) []*core.Node {
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].Id < nodes[j].Id
	})
	return nodes
}

type RewriteManager struct {
	currentNodeId    int
	startVarId       int
	RootNode         *core.Node
	PreNode          *core.Node
	CircleEntryPoint []*core.Node
	WhileNode        []*core.Node
	IfNodes          []*core.Node
	SwitchNode       []*core.Node
	TryNodes         []*core.Node
	DominatorMap     map[*core.Node][]*core.Node
	LabelId          int
	// LoopRegionReducible records whether the ORIGINAL method CFG (before any loop wrapping) is a
	// reducible flow graph. It is computed once in Rewrite() because mid-pipeline the graph gains
	// do-while wrapper nodes and rewrite-inserted break/continue edges that corrupt dominance, making a
	// live-graph reducibility test unreliable. Pre-header pruning in the loop-exit search is enabled only
	// for reducible methods; irreducible methods (e.g. ant CBZip2OutputStream.hbMakeCodeLengths) keep the
	// legacy over-approximated body to avoid collapsing the loop.
	LoopRegionReducible bool
	visitedNodeSet      *utils.Set[*core.Node]
	// Aggressive enables higher-risk structuring paths that only run during the
	// gated second pass for methods that already failed conservative decompilation.
	Aggressive bool
}

func NewRootStatementManager(node *core.Node) *RewriteManager {
	if node == nil {
		return nil
	}
	manager := &RewriteManager{
		RootNode:       node,
		visitedNodeSet: utils.NewSet[*core.Node](),
	}
	return manager
}

func (s *RewriteManager) CheckVisitedNode(node *core.Node) error {
	if s.visitedNodeSet.Has(node) {
		return fmt.Errorf("current node %d has been visited", node.Id)
	}
	s.visitedNodeSet.Add(node)
	return nil
}
func (s *RewriteManager) NewLoopLabel() string {
	s.LabelId++
	return fmt.Sprintf("LOOP_%d", s.LabelId)
}

func (s *RewriteManager) MergeIf() {
	for {
		if !s.mergeIf() {
			break
		}

	}
}

// RemoveDeadEndAssigns detaches assignment nodes that have no successor. An AssignStatement is never
// a legal terminal: a method body always ends in a return/throw, so a `var = expr` node with an empty
// Next set is unreachable-forward dead code. These appear when a value-ternary stored to a local
// (`local = a||b ? X : Y; use(local)`) has its single-use local inlined into the consumer but leaves
// the now-dead store node dangling on a fork (entry -> {dead store, consumer}); the dead branch then
// makes the entry a non-condition node with two successors and ToStatementsFromNode aborts with
// "multiple next". Removing the dead store collapses the fork into the straight-line consumer path.
// Detaching is safe: nothing is reachable FROM the node, so its assigned ref cannot reach any use on
// this path, and any genuine use elsewhere is fed by a different definition.
func (s *RewriteManager) RemoveDeadEndAssigns() {
	for i := 0; i < (1 << 16); i++ {
		var target *core.Node
		core.WalkGraph[*core.Node](s.RootNode, func(n *core.Node) ([]*core.Node, error) {
			if target == nil && n != s.RootNode && len(n.Next) == 0 && len(n.Source) > 0 {
				if _, ok := n.Statement.(*statements.AssignStatement); ok {
					target = n
				}
			}
			return n.Next, nil
		})
		if target == nil {
			return
		}
		for _, src := range slices.Clone(target.Source) {
			src.RemoveNext(target)
		}
		target.RemoveAllSource()
	}
}

// isIdentChar reports whether b can appear inside a Java identifier; used for whole-token matching so
// "var1" never matches inside "var12".
func isIdentChar(b byte) bool {
	return b == '_' || b == '$' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// containsToken reports whether tok appears in s delimited by non-identifier characters (a real
// reference to the local named tok, not an accidental substring of a longer name).
func containsToken(s, tok string) bool {
	if tok == "" {
		return false
	}
	for idx := 0; idx <= len(s)-len(tok); {
		i := strings.Index(s[idx:], tok)
		if i < 0 {
			return false
		}
		i += idx
		before := i == 0 || !isIdentChar(s[i-1])
		after := i+len(tok) >= len(s) || !isIdentChar(s[i+len(tok)])
		if before && after {
			return true
		}
		idx = i + 1
	}
	return false
}

// renderValue renders a JavaValue to source text, swallowing any panic from a value type that needs ctx
// fields unavailable at this stage (returns "" so the caller treats it as "no reference" and declines).
func renderValue(ctx *class_context.ClassContext, value values.JavaValue) (s string) {
	if value == nil {
		return ""
	}
	defer func() { _ = recover() }()
	s = value.String(ctx)
	return s
}

// followArm walks a single-successor, single-predecessor chain from an if-arm entry to the node where
// the two arms reconverge (the merge). It returns the arm's interior nodes and that merge, or (nil,nil)
// if the arm forks, dead-ends, or is joined by an unexpected predecessor before reconverging. If start
// is itself the merge (an arm that flows directly into it) it returns (nil, start).
func followArm(start *core.Node) ([]*core.Node, *core.Node) {
	if len(start.Source) >= 2 {
		return nil, start
	}
	if len(start.Source) != 1 {
		return nil, nil
	}
	path := []*core.Node{}
	cur := start
	for i := 0; i < (1 << 16); i++ {
		path = append(path, cur)
		if len(cur.Next) != 1 {
			return nil, nil
		}
		nxt := cur.Next[0]
		if len(nxt.Source) >= 2 {
			return path, nxt
		}
		if len(nxt.Source) != 1 {
			return nil, nil
		}
		cur = nxt
	}
	return nil, nil
}

// definedLocals collects the rendered names of locals assigned by the AssignStatement nodes on an arm
// path. These anchor the true/false mapping in SplitTernaryReturnArms: a value that uses an arm's locals
// must belong to that arm.
func definedLocals(ctx *class_context.ClassContext, path []*core.Node) []string {
	var out []string
	for _, n := range path {
		if as, ok := n.Statement.(*statements.AssignStatement); ok {
			if ref, ok := as.LeftValue.(*values.JavaRef); ok && ref.Id != nil {
				if name := renderValue(ctx, ref); name != "" {
					out = append(out, name)
				}
			}
		}
	}
	return out
}

// SplitTernaryReturnArms undoes a value-ternary reconstruction that cannot be linearized: a
// `return cond ? A : B` whose arm computes its value through intermediate local stores (e.g. ECJ's
// pre-sized `new StringBuilder(len)` idiom, or a lazy `field = compute()`). Those stores remain as
// statement nodes on the arm; once the condition node is spliced out (its callback folds the boolean
// into the ternary) the arm statements dangle on a fork that ToStatements rejects with "multiple next".
//
// This pass instead keeps the condition as a real if and tail-duplicates the shared terminal return
// into each arm, so each arm runs only its own stores and returns its own value:
// `if (cond) return A; <B-stores>; return B;`. It fires only when the arm-to-value mapping is provably
// correct - verified by which arm's locals each value references, with cross-checks that catch an
// inverted (negated) condition - and declines otherwise, so an ambiguous shape degrades to the prior
// stub rather than risking a silently branch-swapped result. ctx is needed only to render values during
// the reference probe.
func (s *RewriteManager) SplitTernaryReturnArms(ctx *class_context.ClassContext) {
	handled := map[*core.Node]bool{}
	for i := 0; i < (1 << 16); i++ {
		var cond *core.Node
		core.WalkGraph[*core.Node](s.RootNode, func(n *core.Node) ([]*core.Node, error) {
			if cond == nil && !handled[n] {
				if st, ok := n.Statement.(*statements.ConditionStatement); ok && st.Callback != nil && st.Condition != nil && len(n.Next) == 2 && n.TrueNode != nil && n.FalseNode != nil {
					cond = n
				}
			}
			return n.Next, nil
		})
		if cond == nil {
			return
		}
		handled[cond] = true
		s.trySplitTernaryReturn(ctx, cond)
	}
}

func (s *RewriteManager) trySplitTernaryReturn(ctx *class_context.ClassContext, cond *core.Node) bool {
	trueB := cond.TrueNode()
	falseB := cond.FalseNode()
	if trueB == nil || falseB == nil || trueB == falseB {
		return false
	}
	truePath, tMerge := followArm(trueB)
	falsePath, fMerge := followArm(falseB)
	if tMerge == nil || tMerge != fMerge || tMerge == cond {
		return false
	}
	merge := tMerge
	// Duplicating the merge is only safe when the two arms are its only predecessors; a third source
	// would lose its edge to the removed merge.
	if len(merge.Source) != 2 {
		return false
	}
	ret, ok := merge.Statement.(*statements.ReturnStatement)
	if !ok {
		return false
	}
	// The merge's return value is the reconstructed ternary, usually wrapped in one or more SlotValue
	// indirections (the stack slot the merge consumed). Unwrap to the underlying ternary.
	rv := ret.JavaValue
	for {
		sv, ok := rv.(*values.SlotValue)
		if !ok || sv.GetValue() == nil {
			break
		}
		rv = sv.GetValue()
	}
	tern, ok := rv.(*values.TernaryExpression)
	if !ok || tern.TrueValue == nil || tern.FalseValue == nil {
		return false
	}
	trueVars := definedLocals(ctx, truePath)
	falseVars := definedLocals(ctx, falsePath)
	trueValStr := renderValue(ctx, tern.TrueValue)
	falseValStr := renderValue(ctx, tern.FalseValue)
	// Need at least one arm-local to anchor the mapping; a pure value ternary (no arm stores) is left to
	// the normal callback collapse.
	if len(trueVars) == 0 && len(falseVars) == 0 {
		return false
	}
	// Cross-contamination check (catches an inverted/negated condition): the value paired with one arm by
	// the if's true/false convention must NOT reference the OTHER arm's locals.
	for _, d := range trueVars {
		if containsToken(falseValStr, d) {
			return false
		}
	}
	for _, d := range falseVars {
		if containsToken(trueValStr, d) {
			return false
		}
	}
	// Own-reference check: a non-empty arm's value must actually use that arm's locals (otherwise the
	// stores would be orphaned by the split).
	if len(trueVars) > 0 {
		used := false
		for _, d := range trueVars {
			if containsToken(trueValStr, d) {
				used = true
				break
			}
		}
		if !used {
			return false
		}
	}
	if len(falseVars) > 0 {
		used := false
		for _, d := range falseVars {
			if containsToken(falseValStr, d) {
				used = true
				break
			}
		}
		if !used {
			return false
		}
	}
	var end *core.Node
	if len(merge.Next) == 1 {
		end = merge.Next[0]
	}
	rt := s.NewNode(&statements.ReturnStatement{JavaValue: tern.TrueValue})
	rf := s.NewNode(&statements.ReturnStatement{JavaValue: tern.FalseValue})
	trueLast := cond
	if len(truePath) > 0 {
		trueLast = truePath[len(truePath)-1]
	}
	falseLast := cond
	if len(falsePath) > 0 {
		falseLast = falsePath[len(falsePath)-1]
	}
	trueLast.ReplaceNext(merge, rt)
	rt.AddSource(trueLast)
	falseLast.ReplaceNext(merge, rf)
	rf.AddSource(falseLast)
	if end != nil {
		rt.AddNext(end)
		rf.AddNext(end)
	}
	merge.RemoveAllSource()
	merge.RemoveAllNext()
	// Keep the condition as a real if so the if-rewriter structures it; clearing the callback prevents
	// the downstream collapse from splicing it out into the (now discarded) ternary.
	if st, ok := cond.Statement.(*statements.ConditionStatement); ok {
		st.Callback = nil
	}
	return true
}

func (s *RewriteManager) mergeIf() bool {
	ifNodes := utils2.NodeFilter(WalkNodeToList(s.RootNode), func(node *core.Node) bool {
		_, ok := node.Statement.(*statements.ConditionStatement)
		return ok
	})
	ifNodes = NodeDeduplication(ifNodes)
	sort.Slice(ifNodes, func(i, j int) bool {
		return ifNodes[i].Id > ifNodes[j].Id
	})
	result := false
	delNodesSet := utils.NewSet[*core.Node]()
	// isTernaryChainArm reports whether a condition node supplies a DISTINCT nested ternary arm
	// (a right-leaning chain a?:b?:c?: or a structurally-rebuilt tree). Such a node must NOT be
	// folded into a short-circuit &&/|| here: its value flows individually into its own ternary
	// arm, so merging it (the arms converge on the same merge node once their leaf values are
	// extracted, making them look like a short-circuit) collapses several distinct conditions
	// into one and leaves the others' callbacks unfired, leaking an empty stack slot. Genuine
	// short-circuit &&/|| conditions all feed the SAME ternary condition and are NOT marked, so
	// they continue to merge normally.
	isTernaryChainArm := func(n *core.Node) bool {
		cond, ok := n.Statement.(*statements.ConditionStatement)
		return ok && cond.TernaryChainArm
	}
	for _, node := range ifNodes {
		if delNodesSet.Has(node) {
			continue
		}
		if isTernaryChainArm(node) {
			continue
		}
		var nextStNode *core.Node
		if len(utils.NewSet(node.Next).List()) == 1 {
			if node.Next[0].SourceConditionNode != node {
				continue
			}
			nextStNode = node.Next[0]
		}

		for _, n := range node.Source {
			mergeCondition := func(parentNode, childNode *core.Node) {
				// Guard against nil TrueNode/FalseNode: when the variable-fold nil-key error is
				// suppressed (parser.go) or empty if-merge sources are tolerated (code_analyser.go),
				// a ConditionStatement whose node has fewer than 2 Next edges can reach here with nil
				// branch functions. Skip the merge instead of panicking. The guard must run BEFORE
				// result is set: setting result=true and then returning without mutating the graph
				// makes MergeIf()'s fixpoint loop re-discover the same unmergeable pair forever (the
				// graph never changes yet mergeIf keeps reporting progress), so it never converges.
				if parentNode == nil || childNode == nil ||
					parentNode.TrueNode == nil || childNode.TrueNode == nil ||
					parentNode.TrueNode() == nil || childNode.TrueNode() == nil {
					return
				}
				result = true
				// Guard against nil TrueNode/FalseNode: when the variable-fold nil-key error is
				// suppressed (parser.go), a ConditionStatement whose node has fewer than 2 Next edges
				// can reach here with nil branch functions. Skip the merge instead of panicking.
				if parentNode == nil || childNode == nil ||
					parentNode.TrueNode == nil || childNode.TrueNode == nil ||
					parentNode.TrueNode() == nil || childNode.TrueNode() == nil {
					return
				}
				if parentNode.TrueNode() != childNode { // or logic
					if parentNode.TrueNode() == childNode.TrueNode() { // same direction
						ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
						ifStat2 := childNode.Statement.(*statements.ConditionStatement)
						ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, ifStat2.Condition, "||", types.NewJavaPrimer(types.JavaBoolean))
						trueNode := parentNode.TrueNode()
						parentNode.RemoveAllNext()
						childFalseNode := childNode.FalseNode()
						childNode.RemoveAllNext()
						parentNode.AddNext(childFalseNode)
						parentNode.AddNext(trueNode)
						delNodesSet.Add(childNode)
					} else {
						ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
						ifStat2 := childNode.Statement.(*statements.ConditionStatement)
						ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, values.NewUnaryExpression(ifStat2.Condition, "!", types.NewJavaPrimer(types.JavaBoolean)), "||", types.NewJavaPrimer(types.JavaBoolean))
						trueNode := parentNode.TrueNode()
						parentNode.RemoveAllNext()
						childTrueNode := childNode.TrueNode()
						childNode.RemoveAllNext()
						parentNode.AddNext(childTrueNode)
						parentNode.AddNext(trueNode)
						delNodesSet.Add(childNode)
					}
				} else { // and logic: if true node is next if node
					if parentNode.FalseNode() == childNode.FalseNode() { // same direction
						ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
						ifStat2 := childNode.Statement.(*statements.ConditionStatement)
						ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, ifStat2.Condition, "&&", types.NewJavaPrimer(types.JavaBoolean))
						falseNode := parentNode.FalseNode()
						parentNode.RemoveAllNext()
						childTrueNode := childNode.TrueNode()
						childNode.RemoveAllNext()
						parentNode.AddNext(falseNode)
						parentNode.AddNext(childTrueNode)
						delNodesSet.Add(childNode)
					} else {
						ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
						ifStat2 := childNode.Statement.(*statements.ConditionStatement)
						ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, values.NewUnaryExpression(ifStat2.Condition, "!", types.NewJavaPrimer(types.JavaBoolean)), "&&", types.NewJavaPrimer(types.JavaBoolean))
						falseNode := parentNode.FalseNode()
						parentNode.RemoveAllNext()
						childFalseNode := childNode.FalseNode()
						childNode.RemoveAllNext()
						parentNode.AddNext(falseNode)
						parentNode.AddNext(childFalseNode)
						delNodesSet.Add(childNode)
					}
				}
			}
			if slices.Contains(ifNodes, n) && !delNodesSet.Has(n) && !isTernaryChainArm(n) {
				parentNode := n
				childNode := node
				if parentNode.Id >= childNode.Id {
					continue
				}
				var ok bool
				for _, n2 := range parentNode.Next {
					ok = ok || slices.Contains(childNode.Next, n2)
				}
				if !ok {
					continue
				}
				// A whole-function dominator tree used to be rebuilt here, inside this
				// doubly-nested loop run to a fixpoint, to feed the CalcEnd calls below.
				// Those calls are disabled, and s.DominatorMap is unconditionally
				// recomputed in Rewrite()/ScanCoreInfo() before any reader, so this was
				// dead computation -- yet it dominated decompile CPU for if-heavy classes
				// (~28% of total on a worst case, e.g. AggregateOperations). The merge
				// decision below depends only on the local graph shape (CheckCanBeMerge),
				// not on dominance, so it is correct to drop it entirely.
				sourceSet := utils.NewSet[*core.Node]()
				sourceSet.AddList(childNode.Source)
				if sourceSet.Len() == 1 && CheckCanBeMerge(parentNode, childNode) {
					if len(childNode.Next) == 1 {
						childNode.Next = append(childNode.Next, childNode.Next[0])
					}
					mergeCondition(parentNode, childNode)
					if nextStNode != nil {
						nextStNode.SourceConditionNode = parentNode
					}
				}
			}
		}
	}
	return result
}
func CheckCanBeMerge(ifNode1, ifNode2 *core.Node) bool {
	// 检查是否一方是另一方的子节点
	var node1, node2 *core.Node
	if slices.Contains(ifNode1.Next, ifNode2) {
		node1 = ifNode1
		node2 = ifNode2
	} else if slices.Contains(ifNode2.Next, ifNode1) {
		node1 = ifNode2
		node2 = ifNode1
	} else {
		return false
	}

	// 获取父节点的另一个子节点
	var otherChild *core.Node
	for _, next := range node1.Next {
		if next != node2 {
			otherChild = next
			break
		}
	}
	if otherChild == nil {
		return false
	}

	// 检查另一个子节点的父节点是否为node2
	return slices.Contains(node2.Next, otherChild)
}
func (s *RewriteManager) SetStartVarId(id int) {
	s.startVarId = id
}
func (s *RewriteManager) SetId(id int) {
	s.currentNodeId = id
}
func (s *RewriteManager) NewNode(st statements.Statement) *core.Node {
	node := core.NewNode(st)
	node.Id = s.GetNewNodeId()
	return node
}
func (s *RewriteManager) GetNewNodeId() int {
	s.currentNodeId++
	return s.currentNodeId
}
func (s *RewriteManager) ScanStatementSimple(handle func(node *core.Node) error) error {
	return s.ScanStatement(func(node *core.Node) (error, bool) {
		err := handle(node)
		if err != nil {
			return err, false
		}
		return nil, true
	})
}
func (s *RewriteManager) ScanStatement(handle func(node *core.Node) (error, bool)) error {
	err := core.WalkGraph[*core.Node](s.RootNode, func(node *core.Node) ([]*core.Node, error) {
		err, ok := handle(node)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return node.Next, nil
	})
	return err
}

func (s *RewriteManager) ToStatementsFromNode(node *core.Node, stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
	var ErrHasCircle = errors.New("has circle")
	result := []*core.Node{}
	current := node
	visited := utils.NewSet[*core.Node]()
	for {
		if current == nil {
			break
		}
		if visited.Has(current) {
			return nil, ErrHasCircle
		}
		err := s.CheckVisitedNode(current)
		if err != nil {
			// Node was already visited (shared merge point in a DAG). Instead of
			// failing the entire method, just stop collecting statements here.
			// The visited node will appear once (from whichever path reached it first).
			break
		}
		if stopCheck != nil && !stopCheck(current) {
			break
		}
		visited.Add(current)
		if _, ok := current.Statement.(*statements.MiddleStatement); !ok {
			result = append(result, current)
		}
		if len(current.Next) == 0 {
			break
		}
		if len(current.Next) > 1 {
			// A non-condition node with multiple Next edges after structuring means some control
			// transfer (break/continue exit, or a fork the if/loop rewriter didn't fully consolidate)
			// is still wired as an edge. Instead of failing the ENTIRE method, follow the first
			// (fall-through) edge. The other edges are typically jump targets whose bodies were
			// already structured (break/continue nodes) or stale exits. The syntax-validation safety
			// net catches any corruption. This clears the "multiple next" family (fastjson2 skipString's
			// do-while with multiple exits, readBoolValue's if with unconsolidated exits).
			current = current.Next[0]
			continue
		}
		current = current.Next[0]
	}
	return result, nil
}
func (s *RewriteManager) ToStatements(stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
	s.visitedNodeSet = utils.NewSet[*core.Node]()
	return s.ToStatementsFromNode(s.RootNode, stopCheck)
}

// isMethodExitTerminator reports whether node is an unconditional method exit (return / throw)
// that has no fall-through successor. It is deliberately narrower than statementIsTerminal:
// break/continue are loop-control jumps that DO have a successor (their loop target), so they
// are not treated as exits here. Used by IfRewriter to drop early-return/throw exits from a
// structured if's linear Next edges (keeping only the genuine fall-through continuation).
func isMethodExitTerminator(node *core.Node) bool {
	if node == nil || node.Statement == nil {
		return false
	}
	switch s := node.Statement.(type) {
	case *statements.ReturnStatement:
		return true
	case *statements.CustomStatement:
		txt := strings.TrimSpace(s.String(pruneCtx))
		return strings.HasPrefix(txt, "throw ")
	}
	return false
}

type NodeExtInfo struct {
	PreNodeRoute    *NodeRoute
	AllPreNodeRoute []*NodeRoute
	AllCircleRoute  []*NodeRoute
	CircleRoute     *utils.Set[*core.Node]
}

func (s *RewriteManager) ScanCoreInfo() error {
	s.DominatorMap = GenerateDominatorTree(s.RootNode)
	nodeExtInfo := map[*core.Node]*NodeExtInfo{}
	getNodeInfo := func(node *core.Node) *NodeExtInfo {
		if info, ok := nodeExtInfo[node]; ok {
			return info
		}
		info := &NodeExtInfo{}
		nodeExtInfo[node] = info
		return info
	}
	stack := utils.NewStack[*core.Node]()
	//visited := NewRootNodeRoute()
	circleNodes := []*core.Node{}
	ifNodes := []*core.Node{}
	mergeNodesSet := utils.NewSet[*core.Node]()
	tryNodesSet := utils.NewSet[*core.Node]()

	var walkIfStatement func(node *core.Node, subNodeRoute *NodeRoute)
	walkIfStatement = func(node *core.Node, subNodeRoute *NodeRoute) {
		getNodeInfo(node).PreNodeRoute = subNodeRoute
		stack.Push(node)
		for stack.Len() > 0 {
			current := stack.Pop()
			getNodeInfo(current).AllPreNodeRoute = append(getNodeInfo(current).AllPreNodeRoute, subNodeRoute)
			if m, ok := getNodeInfo(node).PreNodeRoute.Has(current); ok {
				getNodeInfo(node).AllCircleRoute = append(getNodeInfo(node).AllCircleRoute, subNodeRoute)
				route, _ := getNodeInfo(node).PreNodeRoute.HasPre(current)
				if v := getNodeInfo(current).CircleRoute; v != nil {
					getNodeInfo(current).CircleRoute = v.Or(route)
				} else {
					getNodeInfo(current).CircleRoute = route
					core.WalkGraph[*core.Node](current, func(node *core.Node) ([]*core.Node, error) {
						route.Add(node)
						if len(node.Next) > 1 {
							return nil, nil
						}
						return node.Next, nil
					})
				}
				circleNodes = append(circleNodes, current)
				_ = m
				continue
			}
			skip := len(getNodeInfo(current).AllPreNodeRoute) > 1
			getNodeInfo(node).PreNodeRoute.Add(current)
			if skip {
				mergeNodesSet.Add(current)
				current.IsMerge = true
				continue
			}

			if _, ok := current.Statement.(*statements.ConditionStatement); ok {
				current.IsIf = true
				ifNodes = append(ifNodes, current)
				for _, n := range current.Next {
					walkIfStatement(n, subNodeRoute.NewChild(current))
				}
				continue
			} else if v, ok := current.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleSwitch {
				s.SwitchNode = append(s.SwitchNode, current)
				for _, n := range current.Next {
					newRoute := subNodeRoute.NewChild(current)
					newRoute.ConditionNode = nil
					newRoute.SwitchNode = current
					walkIfStatement(n, newRoute)
				}
				continue
			} else if v, ok := current.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleTryStart {
				tryNodesSet.Add(current)
				for _, n := range current.Next {
					newRoute := subNodeRoute.NewChild(current)
					newRoute.ConditionNode = nil
					newRoute.TryNode = current
					walkIfStatement(n, newRoute)
				}
			} else {
				for _, n := range current.Next {
					stack.Push(n)
				}
			}
		}
	}
	subNodeRoute := NewRootNodeRoute()
	walkIfStatement(s.RootNode, subNodeRoute)
	circleNodes = sortNodesByID(utils.NewSet[*core.Node](circleNodes).List())
	//for _, node := range circleNodes {
	//	//mergeNode := funk.Filter(node.Next, func(item *core.Node) bool {
	//	//	return !node.CircleNodesSet.Has(item)
	//	//}).([]*core.Node)
	//	node.MergeNode = node.FalseNode()
	//}
	switchSet := utils.NewSet[*core.Node]()
	switchSet.AddList(s.SwitchNode)
	s.SwitchNode = sortNodesByID(switchSet.List())
	//for _, node := range s.SwitchNode {
	//	caseItemMap := node.Statement.(*statements.MiddleStatement).Data.([]any)[0].(map[int]*core.Node)
	//	itemMap := map[*core.Node]struct{}{}
	//	for _, item := range caseItemMap {
	//		itemMap[item] = struct{}{}
	//	}
	//	for _, n := range s.DominatorMap[node] {
	//		if _, ok := itemMap[n]; !ok {
	//			node.SwitchMergeNode = n
	//			break
	//		}
	//	}
	//}
	//for switchNode, nodes := range switchMergeNodeCandidates {
	//	caseMap := switchNode.Statement.(*statements.MiddleStatement).Data.([]any)[0].(map[int]*core.Node)
	//	allOk := false
	//	for _, node := range nodes {
	//		for _, route := range getNodeInfo(node).AllPreNodeRoute {
	//
	//		}
	//	}
	//}
	s.TryNodes = sortNodesByID(tryNodesSet.List())
	for _, current := range sortNodesByID(mergeNodesSet.List()) {
		for _, nodeMap := range getNodeInfo(current).AllPreNodeRoute {
			if nodeMap.ConditionNode == nil {
				continue
			}
			if nodeMap.ConditionNode.MergeNode != nil {
				continue
			}
			trueNode, falseNode := ifBranchNodes(nodeMap.ConditionNode)
			checkNode := []*core.Node{trueNode, falseNode}
			isPreNode := true
			for _, node := range checkNode {
				if node == nil {
					isPreNode = false
					break
				}
				isPreNode = CheckIsPreNode(getNodeInfo, current, node) && isPreNode
			}
			if isPreNode {
				nodeMap.ConditionNode.MergeNode = current
			}
		}
	}
	//for _, node := range circleNodes {
	//	node := node
	//	node.InCircle = func(n *core.Node) bool {
	//		return CheckIsPreNode(getNodeInfo, node.LoopEndNode, n) && !CheckIsPreNode(getNodeInfo, node, n)
	//	}
	//}
	circleNodeEntryToOutPoint := map[*core.Node]map[*core.Node]*core.Node{}
	for _, circleNodeEntry := range circleNodes {
		outPointMap := map[*core.Node]*core.Node{}
		circleNodeEntry.CircleNodesSet = getNodeInfo(circleNodeEntry).CircleRoute
		core.WalkGraph(circleNodeEntry, func(node *core.Node) ([]*core.Node, error) {
			next := funk.Filter(node.Next, func(n *core.Node) bool {
				isInCircle := circleNodeEntry.CircleNodesSet.Has(n)
				if n == circleNodeEntry {
					isInCircle = true
				}
				if !isInCircle {
					outPointMap[node] = n
					//outPoint = append(outPoint, n)
				}
				return isInCircle
			}).([]*core.Node)
			circleNodeEntry.CircleNodesSet.Add(node)
			return next, nil
		})
		//if len(outPointMap) == 0 {
		//	return errors.New("invalid circle")
		//}
		tryNodes := funk.Filter(maps.Keys(outPointMap), func(node *core.Node) bool {
			if v, ok := node.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleTryStart {
				return true
			}
			return false
		}).([]*core.Node)
		for _, node := range tryNodes {
			delete(outPointMap, node)
		}
		circleNodeEntry.OutNodeMap = outPointMap
		circleNodeEntryToOutPoint[circleNodeEntry] = outPointMap
	}
	// Iterate the loop entries (and their out-points / out-edge keys) in a stable id order: both the
	// entry map and outPointMap are Go maps, so their natural range order varies per run and would make
	// merge-node selection and ConditionNode ordering non-deterministic.
	for _, circleNodeEntry := range circleNodes {
		outPointMap := circleNodeEntryToOutPoint[circleNodeEntry]
		if outPointMap == nil {
			continue
		}
		var mergeNode *core.Node
		edgeSet := utils.NewSet[*core.Node]()
		values := sortNodesByID(utils.NewSet[*core.Node](maps.Values(outPointMap)).List())
		if len(values) == 0 {
		} else if len(values) == 1 {
			mergeNode = values[0]
		} else {
			core.WalkGraph[*core.Node](values[0], func(node *core.Node) ([]*core.Node, error) {
				edgeSet.Add(node)
				return node.Next, nil
			})
			core.WalkGraph[*core.Node](values[1], func(node *core.Node) ([]*core.Node, error) {
				if edgeSet.Has(node) {
					mergeNode = node
					return nil, nil
				}
				return node.Next, nil
			})
		}
		for _, c := range sortNodesByID(maps.Keys(outPointMap)) {
			check := func(node *core.Node) bool {
				if _, ok := c.Statement.(*statements.ConditionStatement); ok {
					return true
				}
				return false
			}
			if check(c) {
				circleNodeEntry.ConditionNode = append(circleNodeEntry.ConditionNode, c)
			}
		}
		circleNodeEntry.GetLoopEndNode = func() *core.Node {
			return nil
		}
		circleNodeEntry.SetLoopEndNode = func(node *core.Node, node2 *core.Node) {

		}
		loopEndNodeMap := map[*core.Node]*core.Node{}
		if mergeNode != nil {
			loopEndNodeMap[mergeNode] = mergeNode
			circleNodeEntry.GetLoopEndNode = func() *core.Node {
				return loopEndNodeMap[mergeNode]
			}
			circleNodeEntry.SetLoopEndNode = func(node1, node2 *core.Node) {
				loopEndNodeMap[node1] = node2
			}
		}
	}

	// Family B (merge-condition leak): the route-based DFS above marks any node reached by more than
	// one route as a merge and excludes it from ifNodes. A ConditionStatement that is also such a
	// control-flow join therefore never becomes an IfStatement and leaks into the output as a bare
	// `if (cond);`. When such a condition heads a clean single-entry-single-exit region -- both arms
	// reconverge at its immediate post-dominator -- and it is not a loop head or switch/try head, it
	// is a perfectly ordinary if and must be structured. The post-dominator gives a sound region exit
	// even though the head is itself a join (the forward dominator tree alone could not bound it).
	collectSESEMergeConditions(s, &ifNodes, mergeNodesSet, circleNodes)

	s.CircleEntryPoint = circleNodes
	s.IfNodes = ifNodes
	//for _, node := range circleNodes {
	//	for _, c := range s.DominatorMap[node] {
	//		if !node.CircleNodesSet.Has(c) {
	//			node.LoopEndNode = c
	//		}
	//	}
	//}
	return nil
}

// collectSESEMergeConditions augments ifNodes with merge-conditions (ConditionStatement nodes that
// the route-based scan flagged as control-flow joins) that head a clean single-entry-single-exit
// region. See the call site in ScanCoreInfo for the rationale. The trigger is deliberately
// conservative -- a clear post-dominator reconvergence and no loop/switch involvement -- so already
// well-structured methods are untouched and only the previously-leaking bare conditions are promoted.
func collectSESEMergeConditions(s *RewriteManager, ifNodes *[]*core.Node, mergeNodesSet *utils.Set[*core.Node], circleNodes []*core.Node) {
	merges := mergeNodesSet.List()
	if len(merges) == 0 {
		return
	}
	ifNodeSet := utils.NewSet[*core.Node]()
	ifNodeSet.AddList(*ifNodes)
	circleSet := utils.NewSet[*core.Node]()
	circleSet.AddList(circleNodes)

	postDom := GeneratePostDominatorMap(s.RootNode)

	for _, current := range sortNodesByID(merges) {
		if ifNodeSet.Has(current) {
			continue
		}
		if _, ok := current.Statement.(*statements.ConditionStatement); !ok {
			continue
		}
		// Skip anything tangled with a loop: loop heads and any node inside a detected circle are
		// handled by the loop rewriter, and treating a back-edge join as a plain if would be wrong.
		if circleSet.Has(current) {
			continue
		}
		inCircle := false
		for _, c := range circleNodes {
			if c.CircleNodesSet != nil && c.CircleNodesSet.Has(current) {
				inCircle = true
				break
			}
		}
		if inCircle {
			continue
		}
		if current.TrueNode == nil || current.FalseNode == nil {
			continue
		}
		tn := current.TrueNode()
		fn := current.FalseNode()
		if tn == nil || fn == nil {
			continue
		}
		// A self-referential post-dominator means a loop-like shape we must not treat as a plain if.
		exit := postDom[current]
		if exit == current {
			continue
		}
		// A nil post-dominator means one arm exits the method (return/throw) without reconverging.
		// That is still an ordinary if (the `if (!$assertionsDisabled && cond) throw ...` assert idiom
		// is the canonical case), handled fine by IfRewriter's end-node logic. But promoting it
		// unconditionally exposes order-sensitive structuring that regresses passing methods, so it is
		// gated behind aggressive mode: only a method that already failed conservatively takes this path.
		if exit == nil && !s.Aggressive {
			continue
		}
		// Degenerate condition whose both arms jump to the same node is not a real two-armed if.
		if tn == fn {
			continue
		}
		current.IsIf = true
		*ifNodes = append(*ifNodes, current)
		ifNodeSet.Add(current)
	}
}

func (s *RewriteManager) Rewrite() error {
	err := s.ScanCoreInfo()
	if err != nil {
		return err
	}
	// Reducibility must be measured on the pristine CFG, before RebuildLoopNode introduces do-while
	// wrapper nodes (Id 0) and later passes splice in break/continue edges — both distort dominance.
	s.LoopRegionReducible = isReducibleCFG(s.RootNode, GenerateDominatorTree(s.RootNode))
	err = RebuildLoopNode(s)
	if err != nil {
		return err
	}
	s.DominatorMap = GenerateDominatorTree(s.RootNode)
	nodeToRewriter := map[*core.Node]rewriterFunc{}
	keyNodes := []*core.Node{}
	for _, node := range s.WhileNode {
		nodeToRewriter[node] = LoopRewriter
		keyNodes = append(keyNodes, node)
	}
	for _, node := range s.SwitchNode {
		nodeToRewriter[node] = SwitchRewriter
		keyNodes = append(keyNodes, node)
	}
	for _, node := range s.IfNodes {
		nodeToRewriter[node] = IfRewriter
		keyNodes = append(keyNodes, node)
	}
	for _, node := range s.TryNodes {
		nodeToRewriter[node] = TryRewriter
		keyNodes = append(keyNodes, node)
	}

	lo.ForEach(WalkNodeToList(s.RootNode), func(item *core.Node, index int) {
		if v, ok := item.Statement.(*statements.MiddleStatement); ok {
			if v.Flag == "monitor_enter" {
				nodeToRewriter[item] = SynchronizeRewriter
				keyNodes = append(keyNodes, item)
			}
		}
	})
	for _, node := range s.TopologicalSortReverse(s.SwitchNode) {
		err := SwitchRewriter1(s, node)
		if err != nil {
			return err
		}
		s.DominatorMap = GenerateDominatorTree(s.RootNode)
	}
	order := s.TopologicalSortReverse(keyNodes)
	loopJmpRewriterRecoed := map[*core.Node]struct{}{}
	processed := map[*core.Node]struct{}{}
	for i := 0; i < len(order); i++ {
		s.DominatorMap = GenerateDominatorTree(s.RootNode)
		node := order[i]
		if _, done := processed[node]; done {
			continue
		}

		// Family B (merge-condition inside a container body): TryRewriter (and, in aggressive mode,
		// IfRewriter) collects its body's statements via a dominator walk WITHOUT recursively
		// structuring them, so any if-node that lives in the body must be turned into an IfStatement
		// BEFORE the container runs. The reverse-topo order normally guarantees inner-first, but a
		// body entry that is also a control-flow join (a merge-condition) is dominated by a node
		// OUTSIDE the container, so it sorts AFTER the container and would be grabbed raw -> leaked as
		// a bare `if(cond);`. Pre-structure those pending inner if-nodes here.
		//
		// For try containers this runs unconditionally (the try body's linear collection genuinely
		// needs it and it is zero-regression). Extending it to if containers regressed passing
		// methods catastrophically when applied globally (5->318), so it is gated behind aggressive
		// mode: only a method that already failed conservatively takes the if-container path.
		isTry := slices.Contains(s.TryNodes, node)
		isAggrIf := s.Aggressive && slices.Contains(s.IfNodes, node)
		if isTry || isAggrIf {
			body := s.containerBodyNodeSet(node)
			for _, cand := range order[i+1:] {
				if _, done := processed[cand]; done {
					continue
				}
				if cand == node || !body[cand] || !slices.Contains(s.IfNodes, cand) {
					continue
				}
				s.DominatorMap = GenerateDominatorTree(s.RootNode)
				if err := IfRewriter(s, cand); err != nil {
					return err
				}
				processed[cand] = struct{}{}
			}
			s.DominatorMap = GenerateDominatorTree(s.RootNode)
		}

		if slices.Contains(s.IfNodes, node) {
			for j := i; j < len(order); j++ {
				n := order[j]
				if slices.Contains(s.WhileNode, n) && utils2.IsDominate(s.DominatorMap, n, node) {
					if _, ok := loopJmpRewriterRecoed[n]; ok {
						break
					}
					err := LoopJmpRewriter(s, n)
					if err != nil {
						return err
					}
					loopJmpRewriterRecoed[n] = struct{}{}
					s.DominatorMap = GenerateDominatorTree(s.RootNode)
					break
				}
			}
		}
		err := nodeToRewriter[node](s, node)
		if err != nil {
			return err
		}
		processed[node] = struct{}{}
	}
	return nil
}

// containerBodyNodeSet returns the set of nodes a container node (try / if) collects as its body via
// the dominator walk used by TryRewriter.getBody and IfRewriter.getBody: each successor plus the
// nodes reachable from it through dominator edges. It is used to find inner if-nodes that must be
// structured before the container is rewritten.
func (s *RewriteManager) containerBodyNodeSet(containerNode *core.Node) map[*core.Node]bool {
	set := map[*core.Node]bool{}
	for _, start := range containerNode.Next {
		core.WalkGraph[*core.Node](start, func(n *core.Node) ([]*core.Node, error) {
			set[n] = true
			var next []*core.Node
			for _, c := range n.Next {
				if slices.Contains(s.DominatorMap[n], c) {
					next = append(next, c)
				}
			}
			return next, nil
		})
	}
	return set
}

func (s *RewriteManager) TopologicalSortReverse(nodes []*core.Node) []*core.Node {
	order := []*core.Node{}
	nodesMap := map[*core.Node]struct{}{}
	for _, node := range nodes {
		nodesMap[node] = struct{}{}
	}
	core.WalkGraph[*core.Node](s.RootNode, func(node *core.Node) ([]*core.Node, error) {
		if _, ok := nodesMap[node]; ok {
			order = append(order, node)
		}
		return s.DominatorMap[node], nil
	})
	slices.Reverse(order)
	return order
}
func (s *RewriteManager) DumpDominatorTree() {
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	toString := func(node *core.Node) string {
		s := strings.Replace(node.Statement.String(&class_context.ClassContext{}), "\"", "", -1)
		s = strings.Replace(s, "\n", " ", -1)
		return s
	}
	for node, dom := range s.DominatorMap {
		for _, n := range dom {
			sb.WriteString(fmt.Sprintf("\"%d%s\" -> \"%d%s\"\n", node.Id, toString(node), n.Id, toString(n)))
		}
	}
	sb.WriteString("}\n")
	println(sb.String())
}
