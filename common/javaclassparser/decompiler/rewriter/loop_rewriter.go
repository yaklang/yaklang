package rewriter

import (
	"os"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils3 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type LoopStatement struct {
	Condition values.JavaValue
	BodyStart *core.Node
}

func RebuildLoopNode(manager *RewriteManager) error {
	for _, node := range manager.CircleEntryPoint {
		doWhileSt := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
		doWhileNode := manager.NewNode(doWhileSt)
		// Redirect every edge `source -> circleNode` to `source -> doWhileNode` while preserving the
		// edge's index in source.Next. The previous remove-all-source + AddSource approach appended
		// the redirected edge to the end of source.Next; for a bottom-tested loop the loop-condition
		// node is itself a back-edge source, so its branch to circleNode was shoved to index 1,
		// swapping the if's two successors and inverting the reconstructed do-while condition (break
		// and continue landed on the wrong branch). In-place replacement keeps branch polarity.
		for _, n := range slices.Clone(node.Source) {
			replaceNextInPlace(n, node, doWhileNode)
		}
		doWhileNode.AddNext(node)
		manager.WhileNode = append(manager.WhileNode, doWhileNode)
	}
	return nil
}

// replaceNextInPlace rewires the edge node->oldNext to node->newNext while keeping the edge at
// its original index inside node.Next. Position matters: a ConditionStatement's TrueNode()/
// FalseNode() are bound to fixed Next indices computed during graph construction, so appending a
// freshly created break/continue node (the old RemoveNext + AddNext pair) would shift it to the
// other branch and silently invert the loop condition - body and exit swap, producing
// "if (i < n) break; else { body }" instead of "if (i < n) { body } else break;". Replacing in
// place preserves the branch polarity so the reconstructed loop keeps its original semantics.
func replaceNextInPlace(node, oldNext, newNext *core.Node) {
	idx := slices.Index(node.Next, oldNext)
	node.RemoveNext(oldNext)
	if idx < 0 || idx > len(node.Next) {
		node.AddNext(newNext)
		return
	}
	node.Next = slices.Insert(node.Next, idx, newNext)
	// newNext is already spliced into node.Next; AddNext only fixes the reverse Source link here.
	node.AddNext(newNext)
}

// outermostEnclosingLoopWithExit returns the outermost ENCLOSING loop (a while-node that dominates
// circleNode, excluding circleNode itself) whose own loop-exit node equals exitNode, or nil if no
// enclosing loop shares that exit. "Outermost" = the one that dominates the others, so a break that
// escapes several stacked loops with the same exit target jumps all the way out. Used to turn a bare
// `break` (which would only leave the innermost loop) into a labeled `break LOOP_n` when the exit
// actually lies outside an enclosing loop. Setting JDEC_NO_LOOP_BREAK_LABEL_FIX is the kill-switch.
func outermostEnclosingLoopWithExit(manager *RewriteManager, preWhileNodes []*core.Node, preWhileNodeEnds map[*core.Node]*core.Node, circleNode, exitNode *core.Node) *core.Node {
	if os.Getenv("JDEC_NO_LOOP_BREAK_LABEL_FIX") != "" {
		return nil
	}
	var best *core.Node
	for _, m := range preWhileNodes {
		if m == circleNode {
			continue
		}
		if preWhileNodeEnds[m] != exitNode {
			continue
		}
		if best == nil || utils.IsDominate(manager.DominatorMap, m, best) {
			best = m
		}
	}
	return best
}

func LoopJmpRewriter(manager *RewriteManager, circleNode *core.Node) error {
	loopEnd := searchCircleEndNode(circleNode, circleNode.Next[0], manager.DominatorMap, manager.LoopRegionReducible)
	preWhileNodes := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
		return utils.IsDominate(manager.DominatorMap, node, circleNode)
	})
	preWhileNodeEnds := map[*core.Node]*core.Node{}
	for _, n := range preWhileNodes {
		preWhileNodeEnds[n] = searchCircleEndNode(n, n.Next[0], manager.DominatorMap, manager.LoopRegionReducible)
	}
	checkNode := func(node *core.Node) ([]*core.Node, error) {
		if node.IsJmp {
			return nil, nil
		}
		if _, ok := node.Statement.(*statements.IfStatement); ok {
			return nil, nil
		}
		if !utils.IsDominate(manager.DominatorMap, circleNode, node) {
			return nil, nil
		}
		nextList := []*core.Node{}
		allNext := slices.Clone(node.Next)
		for _, next := range allNext {
			// An exception-handler entry (catch / finally-desugar / try-with-resources suppress handler)
			// is reached ONLY via the exception edge, never by normal loop control flow. LoopJmpRewriter
			// must not rewrite that edge into a break/continue: doing so deletes the handler from the
			// enclosing try node's successor list, so the later TryRewriter can no longer wrap the loop
			// body in try/catch - the handler is emitted as dangling post-loop code and the
			// caught-exception placeholder leaks as a bare `Exception` token (Bug U, observed on a
			// try-with-resources whose body is a loop). Leave the edge intact and do NOT descend into the
			// handler (it is not loop body). Kill-switch: JDEC_LOOP_KEEP_CATCH_EDGE_OFF=1.
			if next.IsCatchStart && os.Getenv("JDEC_LOOP_KEEP_CATCH_EDGE_OFF") == "" {
				continue
			}
			if next == circleNode {
				continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "continue"
				}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
				}))
				continueNode.IsJmp = true
				replaceNextInPlace(node, next, continueNode)
				continueNode.AddNext(next)
				continue
			}

			if false && !utils.IsDominate(manager.DominatorMap, node, next) && node != circleNode {
				if node != circleNode {
					breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
						return "break"
					}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
					}))
					breakNode.HideNext = next
					breakNode.IsJmp = true
					node.RemoveNext(next)
					node.AddNext(breakNode)
					breakNode.AddNext(circleNode)
					circleNode.AddNext(next)
					continue
				}
				breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "break"
				}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
				}))
				breakNode.HideNext = next
				breakNode.IsJmp = true

				matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
					return node == next
				})
				if len(matched) > 0 {
					if utils.IsDominate(manager.DominatorMap, matched[0], circleNode) {
						loopNode := matched[0].Statement.(*statements.DoWhileStatement)
						if loopNode.Label == "" {
							label := manager.NewLoopLabel()
							loopNode.Label = label
						}
						breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
							return "continue " + loopNode.Label
						}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
						})
					}
					//} else {
					//	return nil, errors.New("loop end node conflict")
					//}
				} else {
					//var ok bool
					for _, n := range manager.WhileNode {
						if loopEnd == next && utils.IsDominate(manager.DominatorMap, n, circleNode) {
							loopNode := n.Statement.(*statements.DoWhileStatement)
							if loopNode.Label == "" {
								label := manager.NewLoopLabel()
								loopNode.Label = label
							}
							breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return "break " + loopNode.Label
							}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
							})
							//ok = true
							break
						}
					}
					//if !ok {
					//	return nil, errors.New("loop end node conflict")
					//}
				}

				node.RemoveNext(next)
				node.AddNext(breakNode)
				breakNode.AddNext(next)
				continue
			}
			// When the tight loop-exit search returns an ENCLOSING loop's header (a while-node that
			// dominates circleNode), the edge node->next is a back edge to that outer loop, i.e. a
			// `continue LOOP_n`, never a break. This only became reachable once the reducible-method exit
			// search (excludePreHeader) started reporting the true tight exit: for a nested loop whose inner
			// body falls straight back to the outer header (see TestNestedLoop), loopEnd is now the outer
			// do-while node itself. Skip the plain-break branch here so the labeled-continue handling below
			// (the matched-while-node path) runs and emits `continue LOOP_n` instead of a bare `break`.
			nextIsEnclosingLoopHeader := slices.Contains(manager.WhileNode, next) &&
				utils.IsDominate(manager.DominatorMap, next, circleNode)
			if loopEnd != nil && (next == loopEnd && node != circleNode) && !nextIsEnclosingLoopHeader {
				// Shared-exit nested loops: this loop's computed exit (loopEnd) can coincide with an
				// ENCLOSING loop's exit. javac compiles `do { ... while(inner) ... } while(outer)` where the
				// post-test of an inner while is absorbed into the inner do-while(true) body; the only way
				// out of the inner loop then targets a node that is ALSO outside the outer loop. A bare
				// `break` exits only the inner do-while and falls through to the bottom of the outer body,
				// so the outer do-while(true) loops forever and the post-loop code is unreachable (javac:
				// "unreachable statement"). When the exit escapes an enclosing loop, emit a labeled
				// `break LOOP_n` targeting the OUTERMOST loop whose exit is this same node instead.
				if enclosing := outermostEnclosingLoopWithExit(manager, preWhileNodes, preWhileNodeEnds, circleNode, next); enclosing != nil {
					loopNode := enclosing.Statement.(*statements.DoWhileStatement)
					if loopNode.Label == "" {
						loopNode.Label = manager.NewLoopLabel()
					}
					breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
						return "break " + loopNode.Label
					}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
					}))
					breakNode.IsJmp = true
					// Mirror the plain-break wiring but hand the exit edge to the ENCLOSING loop: the break
					// leaf flows to the outer loop node, and the outer loop node owns the edge to the shared
					// exit. This lets the outer LoopRewriter pick `next` up as its exit (endNode) so the
					// post-loop code stays reachable, instead of being dropped as "incomplete control flow".
					replaceNextInPlace(node, next, breakNode)
					breakNode.AddNext(enclosing)
					enclosing.AddNext(next)
					continue
				}
				breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "break"
				}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
				}))
				replaceNextInPlace(node, next, breakNode)
				breakNode.AddNext(circleNode)
				circleNode.AddNext(next)
				breakNode.IsJmp = true
				continue
			}
			if node != circleNode {
				matched := utils.NodeFilter(manager.WhileNode, func(node *core.Node) bool {
					return node == next
				})
				if len(matched) > 0 {
					if utils.IsDominate(manager.DominatorMap, matched[0], circleNode) {
						loopNode := matched[0].Statement.(*statements.DoWhileStatement)
						if loopNode.Label == "" {
							label := manager.NewLoopLabel()
							loopNode.Label = label
						}
						breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
							return "break"
						}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
						}))
						breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
							return "continue " + loopNode.Label
						}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
						})
						breakNode.IsJmp = true
						replaceNextInPlace(node, next, breakNode)
						breakNode.AddNext(matched[0])
					}
					//} else {
					//	return nil, errors.New("loop end node conflict")
					//}
				} else {
					//var ok bool
					for _, n := range manager.WhileNode {
						// Bug O: a labeled `break outer` jumps from inside this (inner) loop to an enclosing
						// loop's exit. Reverse-topological processing structures the inner loop FIRST, so the
						// enclosing do-while still carries only its single wrapper edge (len(Next)==1) at this
						// point; the legacy guard skipped it and the break-outer (plus the assignment right
						// before it) was dropped, leaving the inner loop spinning forever. Relax the guard for
						// reducible methods (preWhileNodeEnds is only populated for genuine enclosing loops, so
						// non-enclosing while-nodes still cannot match). Irreducible methods keep the guard.
						relaxLabelGuard := manager.LoopRegionReducible && os.Getenv("JDEC_NO_LOOP_BREAK_LABEL_FIX") == ""
						if len(n.Next) < 2 && !relaxLabelGuard {
							continue
						}
						endNode := preWhileNodeEnds[n]
						if endNode == next {
							loopNode := n.Statement.(*statements.DoWhileStatement)
							if loopNode.Label == "" {
								label := manager.NewLoopLabel()
								loopNode.Label = label
							}
							breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return "break"
							}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
							}))
							breakNode.Statement = statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return "break " + loopNode.Label
							}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
							})
							breakNode.IsJmp = true
							replaceNextInPlace(node, next, breakNode)
							breakNode.AddNext(endNode)
							break
						}
					}
					//if !ok {
					//	return nil, errors.New("loop end node conflict")
					//}
				}
			}
			nextList = append(nextList, next)
		}
		return nextList, nil
	}
	times := 0
	err := core.WalkGraph[*core.Node](circleNode.Next[0], func(node *core.Node) ([]*core.Node, error) {
		times++
		//return node.Next, nil
		return checkNode(node)
	})
	if err != nil {
		return err
	}
	_, err = checkNode(circleNode)
	if err != nil {
		return err
	}
	return nil
}
func LoopRewriter(manager *RewriteManager, node *core.Node) error {
	circleNode := node
	loopStart := circleNode.Next[0]
	circleNode.RemoveNext(loopStart)

	body := []statements.Statement{}
	endNodes := []*core.Node{}
	circleSet := getCircleElementSet(circleNode, loopStart, manager.DominatorMap)
	err := core.WalkGraph[*core.Node](loopStart, func(node *core.Node) ([]*core.Node, error) {
		if !circleSet.Has(node) {
			endNodes = append(endNodes, node)
			return nil, nil
		}
		err := manager.CheckVisitedNode(node)
		if err != nil {
			// Node was already visited (shared merge point). Skip instead of failing.
			return nil, nil
		}
		body = append(body, node.Statement)
		var next []*core.Node
		for _, n := range node.Next {
			if slices.Contains(manager.DominatorMap[node], n) {
				next = append(next, n)
			} else {
				if n != circleNode {
					endNodes = append(endNodes, n)
				}
			}
		}
		return next, nil
	})
	if err != nil {
		return err
	}
	doWhileSt := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
	doWhileSt.Label = circleNode.Statement.(*statements.DoWhileStatement).Label
	doWhileSt.Body = append(doWhileSt.Body, body...)
	//allSource := slices.Clone(node.Source)
	//node.RemoveAllSource()
	//for _, n := range allSource {
	//	n.AddNext(manager.NewNode(doWhileSt))
	//}
	loopNode := manager.NewNode(doWhileSt)
	circleNode.Replace(loopNode)
	endNodes = lo.Filter(endNodes, func(item *core.Node, index int) bool {
		return !IsEndNode(item)
	})
	for _, c := range NodeDeduplication(endNodes) {
		loopNode.AddNext(c)
	}
	return nil
}
func getCircleElementSet(circleNode *core.Node, loopStart *core.Node, domTree map[*core.Node][]*core.Node) *utils2.Set[*core.Node] {
	return circleElementSet(circleNode, loopStart, domTree, false)
}

// circleElementSet computes the loop body for the loop whose header is circleNode (its original header
// retained as loopStart). It reverse-BFS's from the back-edge sources (nodes whose Next includes
// circleNode), stopping at circleNode, so post-loop code is excluded.
//
// excludePreHeader controls SEED selection. The default (false) seeds from EVERY predecessor of
// circleNode — the historical behavior, kept verbatim for the loop BODY because real-world IRREDUCIBLE
// loops (e.g. ant CBZip2OutputStream.hbMakeCodeLengths) depend on that over-approximation and collapse
// ("has circle") if any seed is dropped. When excludePreHeader is true (used ONLY by the exit/loopEnd
// search of a REDUCIBLE method, see searchCircleEndNode) it drops genuine pre-header FORWARD entry edges
// so the exit of a reducible nested loop is computed from the tight body instead of leaking into the
// enclosing loop.
func circleElementSet(circleNode *core.Node, loopStart *core.Node, domTree map[*core.Node][]*core.Node, excludePreHeader bool) *utils2.Set[*core.Node] {
	finalSet := utils2.NewSet[*core.Node]()
	// Step 1: collect all nodes reachable from loopStart and build reverse adjacency.
	reverseAdj := map[*core.Node][]*core.Node{}
	allNodes := utils2.NewSet[*core.Node]()
	{
		stk := []*core.Node{loopStart}
		for len(stk) > 0 {
			n := stk[len(stk)-1]
			stk = stk[:len(stk)-1]
			if allNodes.Has(n) {
				continue
			}
			allNodes.Add(n)
			for _, next := range n.Next {
				reverseAdj[next] = append(reverseAdj[next], n)
				if !allNodes.Has(next) {
					stk = append(stk, next)
				}
			}
		}
	}
	// Step 2: find back-edge sources — nodes whose Next includes circleNode.
	var allSources []*core.Node
	for _, n := range allNodes.List() {
		if slices.Contains(n.Next, circleNode) {
			allSources = append(allSources, n)
		}
	}
	sources := allSources
	if excludePreHeader && os.Getenv("JDEC_NO_LOOP_BACKEDGE_DOM_FILTER") == "" {
		// Drop forward pre-header entry edges. The caller only sets excludePreHeader for a method whose
		// ORIGINAL CFG is reducible (see RewriteManager.LoopRegionReducible), so a forward, non-dominated
		// predecessor is a genuine pre-header rather than an alternate entry of an irreducible tangle.
		// circleNode is the freshly-built do-while node (Id 0); loopStart is the ORIGINAL header and still
		// carries its bytecode Id, so a real back edge is a BACKWARD jump (source.Id >= loopStart.Id) or a
		// dominated tail, while a pre-header is a FORWARD non-dominated edge.
		var kept []*core.Node
		for _, n := range allSources {
			if n.Id >= loopStart.Id || utils.IsDominate(domTree, circleNode, n) {
				kept = append(kept, n)
			}
		}
		if len(kept) > 0 {
			sources = kept
		}
	}
	finalSet = reverseBFSStopAt(sources, reverseAdj, circleNode)
	return finalSet
}

// isReducibleCFG reports whether the (pristine) CFG rooted at root is a reducible flow graph, using the
// Dragon-book characterization: partition edges into back edges and forward edges, where a back edge is
// any m->s whose head s DOMINATES its tail m; the graph is reducible iff the remaining forward edges
// form a DAG (removing the back edges leaves no cycle).
//
// We must NOT approximate "back edge" by bytecode/Id order: node Id is a per-node ordinal that does not
// honour control-flow topology — control-flow SINKS like the synthetic `end` node and early `return`
// nodes get LOW Ids even though every edge into them is a forward edge to a terminal. An Id-based
// "retreating edge" test therefore flags e.g. `return var -> end` as retreating and, because `end` does
// not dominate the return, misreports a perfectly reducible nested loop with an early return (Sieve of
// Eratosthenes guarded by `if (n < 2) return 0;`) as irreducible. Dominance-based back-edge detection
// avoids that entirely.
//
// Must be called BEFORE RebuildLoopNode so every node is an original node and the dominator relation is
// not yet perturbed by do-while wrappers or break/continue edges.
func isReducibleCFG(root *core.Node, domTree map[*core.Node][]*core.Node) bool {
	allNodes := []*core.Node{}
	seen := map[*core.Node]bool{}
	core.WalkGraph[*core.Node](root, func(n *core.Node) ([]*core.Node, error) {
		if !seen[n] {
			seen[n] = true
			allNodes = append(allNodes, n)
		}
		return n.Next, nil
	})
	// forward = every edge except back edges (head dominates tail; this also covers self loops because a
	// node always dominates itself).
	forward := map[*core.Node][]*core.Node{}
	for _, m := range allNodes {
		for _, s := range m.Next {
			if utils.IsDominate(domTree, s, m) {
				continue
			}
			forward[m] = append(forward[m], s)
		}
	}
	// Iterative three-colour DFS cycle detection over the forward subgraph (iterative to avoid blowing
	// the stack on large methods). A grey-target edge means a remaining cycle => irreducible.
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[*core.Node]int{}
	type frame struct {
		node *core.Node
		idx  int
	}
	for _, start := range allNodes {
		if color[start] != white {
			continue
		}
		color[start] = grey
		stack := []frame{{start, 0}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			succ := forward[top.node]
			if top.idx < len(succ) {
				s := succ[top.idx]
				top.idx++
				switch color[s] {
				case grey:
					return false
				case white:
					color[s] = grey
					stack = append(stack, frame{s, 0})
				}
			} else {
				color[top.node] = black
				stack = stack[:len(stack)-1]
			}
		}
	}
	return true
}

// reverseBFSStopAt returns {circleNode} ∪ all nodes that can reach a seed via reverse edges without
// passing through circleNode.
func reverseBFSStopAt(seeds []*core.Node, reverseAdj map[*core.Node][]*core.Node, circleNode *core.Node) *utils2.Set[*core.Node] {
	set := utils2.NewSet[*core.Node]()
	set.Add(circleNode)
	queue := []*core.Node{}
	for _, n := range seeds {
		if !set.Has(n) {
			set.Add(n)
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, pred := range reverseAdj[n] {
			if pred != circleNode && !set.Has(pred) {
				set.Add(pred)
				queue = append(queue, pred)
			}
		}
	}
	return set
}

func searchCircleEndNode(circleNode *core.Node, loopStart *core.Node, domTree map[*core.Node][]*core.Node, reducible bool) *core.Node {
	// For a reducible method, use the pre-header-excluding body so a nested loop's exit is computed from
	// its tight body (Bug Q): with the legacy over-approximated body, the reverse-BFS leaks through this
	// loop's exit into the enclosing loop, making the only out-node the OUTER exit, so the inner loop
	// never gets its `else{break}` and becomes a non-terminating do-while(true). Irreducible methods keep
	// the legacy body to avoid collapsing the loop. The LOOP BODY itself always stays on the legacy path
	// (getCircleElementSet); only this exit search prunes pre-headers.
	elementSet := circleElementSet(circleNode, loopStart, domTree, reducible)
	outNodes := []*core.Node{}
	elementSet.ForEach(func(node *core.Node) {
		for _, n := range node.Next {
			if !elementSet.Has(n) {
				// An exception-handler entry (catch / finally-desugar / try-with-resources suppress
				// handler) is reached ONLY via the exception edge, never as a normal loop exit. Counting
				// it as an out-edge fabricates a spurious second exit for a loop whose body contains a
				// try-start: the generic multi-exit merge below then collapses the real fall-out exit and
				// the handler into their shared post-dominator (typically the method's `return`), so
				// loopEnd is wrong, the normal exit edge never becomes a `break`, and the loop degrades to
				// a non-terminating do-while(true) with the post-loop continuation absorbed into the body
				// (Bug U second form: try-with-resources + finally whose body is a loop). Exclude handler
				// edges so the tight fall-out exit is found. Kill-switch: JDEC_LOOP_KEEP_CATCH_EDGE_OFF=1.
				if n.IsCatchStart && os.Getenv("JDEC_LOOP_KEEP_CATCH_EDGE_OFF") == "" {
					continue
				}
				outNodes = append(outNodes, n)
			}
		}
	})
	outNodes = NodeDeduplication(outNodes)
	if len(outNodes) == 0 {
		return nil
	}
	if len(outNodes) == 1 {
		return outNodes[0]
	}
	// Bug O — multi-exit loop: a nested loop that carries a labeled break/continue to an enclosing loop
	// has more than one out-edge (its own fall-out exit PLUS the secondary jump that escapes outward).
	// The generic merge below collapses those exits into their common post-dominator, which for
	// `for{ for{ if(..) continue outer; if(..){found=..; break outer;} } }` is the method's return, not
	// the inner loop's real exit — so the inner loop loses its fall-out target and the break-outer's
	// preceding assignment is dropped. Prefer the loop HEADER's own exit edge: the unique successor of
	// the header (loopStart) that lies OUTSIDE the tight loop body is the canonical while-style fall-out
	// exit, and the remaining out-edges are then correctly classified as labeled break/continue by
	// LoopJmpRewriter. Gated on a reducible method (the header is well-defined) and only when there is
	// genuine multi-exit ambiguity, so single-exit loops are byte-for-byte unchanged.
	if reducible && os.Getenv("JDEC_NO_LOOP_HEADER_EXIT") == "" {
		var headerOut []*core.Node
		for _, n := range loopStart.Next {
			if !elementSet.Has(n) {
				headerOut = append(headerOut, n)
			}
		}
		if len(NodeDeduplication(headerOut)) == 1 {
			return headerOut[0]
		}
	}
	if len(outNodes) > 1 {
		edgeSet := utils2.NewSet[*core.Node]()
		core.WalkGraph[*core.Node](outNodes[0], func(node *core.Node) ([]*core.Node, error) {
			edgeSet.Add(node)
			return node.Next, nil
		})
		var mergeNode *core.Node
		core.WalkGraph[*core.Node](outNodes[1], func(node *core.Node) ([]*core.Node, error) {
			if edgeSet.Has(node) {
				mergeNode = node
				return nil, nil
			}
			return node.Next, nil
		})
		return mergeNode
	}

	return nil
}
