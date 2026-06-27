package rewriter

import (
	"math"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils3 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// renderHead returns the first rendered token of a statement (best effort, no context), used to
// classify control-flow leaves (break/continue/return/throw/goto) emitted as CustomStatements.
func renderHead(st statements.Statement) string {
	if st == nil {
		return ""
	}
	s := strings.TrimSpace(st.String(&class_context.ClassContext{}))
	if idx := strings.IndexAny(s, " \t\n;"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

// isBreakStatement reports whether st is a bare `break` (the leaf used to exit a switch). A break is
// the only abrupt-completion leaf that makes its enclosing switch "complete normally" (control reaches
// the statement after the switch); continue/return/throw transfer control elsewhere.
func isBreakStatement(st statements.Statement) bool {
	return renderHead(st) == "break"
}

// isTerminatorStatement reports whether st abruptly completes (does not fall off its end into the
// following statement): return/throw and the break/continue/goto leaves.
func isTerminatorStatement(st statements.Statement) bool {
	if _, ok := st.(*statements.ReturnStatement); ok {
		return true
	}
	switch renderHead(st) {
	case "break", "continue", "return", "throw", "goto":
		return true
	}
	return false
}

// switchCompletesNormally reports whether control can reach the point immediately after sw (i.e. some
// path leaves the switch by breaking or by falling off the end of a case body). It returns false only
// when sw is exhaustive (has a default) and every arm transfers control away (return/throw/continue)
// so nothing can reach the post-switch point. This gates the nested-switch break repair so we never
// append an unreachable `break` after a switch whose arms all return.
func switchCompletesNormally(sw *statements.SwitchStatement) bool {
	if sw == nil {
		return false
	}
	hasDefault := false
	for _, c := range sw.Cases {
		if c.IsDefault {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		return true // an unmatched value falls through past the switch.
	}
	for _, c := range sw.Cases {
		if len(c.Body) == 0 {
			return true // an empty case falls through to the next arm / past the switch.
		}
		last := c.Body[len(c.Body)-1]
		if isBreakStatement(last) {
			return true
		}
		if !isTerminatorStatement(last) {
			return true // the arm falls off its end, reaching the post-switch point.
		}
	}
	return false
}

func SwitchRewriter1(manager *RewriteManager, node *core.Node) error {
	// manager.DominatorMap = GenerateDominatorTree(manager.RootNode)
	// manager.DumpDominatorTree()
	middleStatement := node.Statement.(*statements.MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseToIndexMap := switchData[0].(*omap.OrderedMap[int, int])
	caseMap := omap.NewEmptyOrderedMap[int, *core.Node]()
	// node.Next is already in case-index order: caseToIndexMap maps each case value to
	// an index into node.Next as captured at parse time. The previous sort.Slice used an
	// invalid comparator (always returning true), which scrambled this slice into an
	// arbitrary permutation and made every case map to the wrong body (and broke the
	// dominator-based merge detection, producing "multiple next"). Keep the original order.
	nexts := slices.Clone(node.Next)
	caseToIndexMap.ForEach(func(k int, v int) bool {
		// Defensive bounds check: the case-to-index map is captured at parse time; if an
		// earlier pass shrank node.Next the index could be stale. Skip the unmappable case
		// instead of panicking the whole method into a stub.
		if v < 0 || v >= len(nexts) {
			return true
		}
		caseMap.Set(k, nexts[v])
		return true
	})
	keyMap := caseMap.Keys()
	sort.Ints(keyMap)
	keyMap = append(keyMap[1:], -1)
	startNodes := caseMap.Values()
	// var mergeNodes []*core.Node
	// for _, startNode := range startNodes {
	// 	core.WalkGraph[*core.Node](startNode, func(n *core.Node) ([]*core.Node, error) {
	// 		// 检查是否所有前驱节点都在当前路径上
	// 		allSourcesInPath := true
	// 		for _, source := range n.Source {
	// 			inPath := false
	// 			core.WalkGraph[*core.Node](startNode, func(pathNode *core.Node) ([]*core.Node, error) {
	// 				if pathNode == source {
	// 					inPath = true
	// 					return nil, nil
	// 				}
	// 				return pathNode.Next, nil
	// 			})
	// 			if !inPath {
	// 				allSourcesInPath = false
	// 				break
	// 			}
	// 		}

	// 		if allSourcesInPath && len(n.Source) > 1 {
	// 			mergeNodes = append(mergeNodes, n)
	// 			return nil, nil
	// 		}
	// 		return n.Next, nil
	// 	})
	// }
	endNodes := utils.NodeFilter(manager.DominatorMap[node], func(node *core.Node) bool {
		if v, ok := node.Statement.(*statements.MiddleStatement); ok && (v.Flag == "end" || v.Flag == "start") {
			return false
		}
		return !slices.Contains(startNodes, node)
	})
	//node.RemoveAllNext()
	endNodes = utils2.NewSet[*core.Node](endNodes).List()
	if len(endNodes) > 1 {
		// A switch with multiple merge targets is a known structuring limitation.
		// Instead of failing, pick the first end node as the merge point. Extra end
		// nodes become break targets. This avoids forcing the method into a stub.
		endNodes = endNodes[:1]
	}
	var mergeNode *core.Node
	if len(endNodes) == 1 {
		mergeNode = endNodes[0]
		allSources := slices.Clone(mergeNode.Source)
		mergeNode.RemoveAllSource()
		for _, source := range allSources {
			breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return "break"
			}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
			}))
			source.AddNext(breakNode)
		}
		node.AddNext(mergeNode)
	}
	if mergeNode == nil {
		mergeNode = caseMap.GetMust(-1)
	}
	node.MergeNode = mergeNode
	return nil
}
func SwitchRewriter(manager *RewriteManager, node *core.Node) error {
	startSwitchNode := node
	SwitchRewriter1(manager, node)
	//switchNode := node
	middleStatement := node.Statement.(*statements.MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseToIndexMap := switchData[0].(*omap.OrderedMap[int, int])
	caseMap := omap.NewEmptyOrderedMap[int, *core.Node]()
	// node.Next is already in case-index order: caseToIndexMap maps each case value to
	// an index into node.Next as captured at parse time. The previous sort.Slice used an
	// invalid comparator (always returning true), which scrambled this slice into an
	// arbitrary permutation and made every case map to the wrong body (and broke the
	// dominator-based merge detection, producing "multiple next"). Keep the original order.
	nexts := slices.Clone(node.Next)
	caseToIndexMap.ForEach(func(k int, v int) bool {
		// Defensive bounds check: the case-to-index map is captured at parse time; if an
		// earlier pass shrank node.Next the index could be stale. Skip the unmappable case
		// instead of panicking the whole method into a stub.
		if v < 0 || v >= len(nexts) {
			return true
		}
		caseMap.Set(k, nexts[v])
		return true
	})
	data := switchData[1].(values.JavaValue)
	//defaultCase := caseMap[-1]
	//delete(caseMap, -1)
	//_ = defaultCase
	caseMapKeys := caseMap.Keys()
	caseMap.Set(math.MaxInt, caseMap.GetMust(-1))
	caseMap.Delete(-1)
	// Map each case value to the ABSOLUTE bytecode offset of its body (Bug G fix). javac lays case
	// bodies out in SOURCE order at increasing offsets and the fall-through edges follow that
	// physical layout, so a switch written with cases in descending value order
	// (case 3: ...; case 2: ...; case 1: ...) has bodies at INCREASING offsets while their VALUES
	// descend. Emitting cases sorted by VALUE (the old behaviour) re-ordered them 1,2,3 and silently
	// inverted the fall-through direction (Murmur3 tail -> array out of bounds / wrong digest).
	// switchData[2] carries SwitchJmpCase (value -> offset, plus -1 for default); fall back to value
	// order when it is absent (e.g. legacy two-element switch data).
	valueToBodyOffset := map[int]int{}
	if len(switchData) >= 3 {
		if offMap, ok := switchData[2].(*omap.OrderedMap[int, int32]); ok && offMap != nil {
			offMap.ForEach(func(v int, off int32) bool {
				if v == -1 {
					// default's offset is keyed under math.MaxInt downstream (caseMap remaps -1).
					valueToBodyOffset[math.MaxInt] = int(off)
				} else {
					valueToBodyOffset[v] = int(off)
				}
				return true
			})
		}
	}
	sort.Ints(caseMapKeys)
	caseItems := []*statements.CaseItem{}
	// case start node source must content switch node
	//breakNode := map[int]*core.Node{}
	//replaceBreakCB := []func(){}
	//statementPatternCheck := []func() bool{}
	//if len(caseMap[-1].Next) == 1 && caseMap[-1].Next[0] == switchNode.SwitchMergeNode {
	//	switchNode.SwitchMergeNode = nil
	//}
	switchStatement := statements.NewSwitchStatement(data, caseItems)
	caseStartNodesMap := map[*core.Node]struct{}{}
	caseMap.ForEach(func(i int, v *core.Node) bool {
		caseStartNodesMap[v] = struct{}{}
		return true
	})
	keyMap := caseMap.Keys()
	node.RemoveAllNext()
	switchNode := manager.NewNode(switchStatement)
	node.Replace(switchNode)
	if node.MergeNode != nil {
		node.RemoveNext(node.MergeNode)
		switchNode.AddNext(node.MergeNode)
	}
	nodeToVals := omap.NewEmptyOrderedMap[*core.Node, []int]()
	caseMap.ForEach(func(i int, v *core.Node) bool {
		idList := nodeToVals.GetMust(v)
		nodeToVals.Set(v, append(idList, i))
		return true
	})
	newNodeToVals := omap.NewEmptyOrderedMap[*core.Node, []int]()
	nodeToVals.ForEach(func(k *core.Node, v []int) bool {
		sort.Ints(v)
		newNodeToVals.Set(k, v)
		for i, val := range v {
			if i == len(v)-1 {
				break
			}
			caseMap.Set(val, nil)
		}
		return true
	})
	nodeToVals = newNodeToVals
	sort.Ints(keyMap)
	var endNodes []*core.Node
	var bodyNodes []*core.Node
	// caseExitsMap records, per case, the CFG nodes its body exits to (the non-dominated successors).
	// It is used after physical-order sorting to decide whether a non-last case actually FALLS THROUGH
	// to the next case or EXITS the switch (and therefore needs an explicit `break`).
	caseExitsMap := map[*statements.CaseItem][]*core.Node{}
	for _, v := range keyMap {
		startNode := caseMap.GetMust(v)
		caseItem := statements.NewCaseItem(v, nil)
		if startNode == nil {
			caseItems = append(caseItems, caseItem)
			continue
		}
		caseItem.IsDefault = v == math.MaxInt
		if caseItem.IsDefault {
			switchNode.RemoveNext(startNode)
		}
		//var terminalIsEndNode bool
		var sts []*core.Node
		var caseExits []*core.Node
		err := core.WalkGraph(startNode, func(node *core.Node) ([]*core.Node, error) {
			if node == startNode {
				if !slices.Contains(manager.DominatorMap[startSwitchNode], node) {
					return nil, nil
				}
			}
			sts = append(sts, node)
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				} else {
					endNodes = append(endNodes, n)
					caseExits = append(caseExits, n)
				}
			}
			return next, nil
		})
		if err != nil {
			return err
		}
		caseItem.Body = lo.Map[*core.Node](sts, func(item *core.Node, index int) statements.Statement {
			return item.Statement
		})
		caseExitsMap[caseItem] = caseExits
		bodyNodes = append(bodyNodes, sts...)
		caseItems = append(caseItems, caseItem)
	}
	switchStatement.Cases = caseItems
	// Order cases by physical body offset to preserve fall-through (Bug G); fall back to value order
	// for any case whose offset is unknown, and use value as a stable tiebreaker so grouped labels
	// sharing one body (same offset) stay value-ascending and adjacent. Keep `default` last as the
	// legacy structuring did: its body is normally the switch merge/exit point, so reordering it by
	// raw offset is unnecessary and risks hoisting it above real cases. Set JDEC_SWITCH_VALUE_ORDER=1
	// to restore the legacy value-only ordering as a kill-switch.
	if os.Getenv("JDEC_SWITCH_VALUE_ORDER") != "" {
		sort.SliceStable(caseItems, func(i, j int) bool {
			return caseItems[i].IntValue < caseItems[j].IntValue
		})
	} else {
		sort.SliceStable(caseItems, func(i, j int) bool {
			di, dj := caseItems[i].IsDefault, caseItems[j].IsDefault
			if di != dj {
				return dj
			}
			if di && dj {
				return false
			}
			pi, oki := valueToBodyOffset[caseItems[i].IntValue]
			pj, okj := valueToBodyOffset[caseItems[j].IntValue]
			if oki && okj && pi != pj {
				return pi < pj
			}
			if oki != okj {
				// A case with a known offset sorts before one without (defensive).
				return oki
			}
			return caseItems[i].IntValue < caseItems[j].IntValue
		})
	}
	// Insert an explicit `break` for any non-last case that ends in a NESTED switch yet EXITS the
	// outer switch instead of falling through to the next case. javac collapses the inner switch's
	// break and the immediately-following outer break into one set of edges to the SHARED exit (there
	// is no instruction between the inner switch and the outer break), so the inner switch's exit
	// reconverges with the outer switch's exit; the dominator-based merge detection cannot attribute
	// that shared point to the inner switch, so the structured inner switch is left without an exit
	// edge and the outer case has neither a break leaf nor a fall-through edge - it silently falls
	// through to the next case label. Detect it structurally and repair it: a non-last case whose body
	// ends in a nested switch that COMPLETES NORMALLY (some arm breaks / falls off, i.e. control can
	// reach the point after the inner switch) and that does NOT fall through to a sibling case must end
	// with a `break`. The nested-switch + completes-normally guards keep this from emitting unreachable
	// code after a loop, a return/throw, or a switch all of whose arms return.
	if os.Getenv("JDEC_SWITCH_NO_BREAK_FIX") == "" {
		for idx, ci := range caseItems {
			if idx == len(caseItems)-1 {
				continue // the last case exits to the merge naturally; no break needed.
			}
			if len(ci.Body) == 0 {
				continue // empty grouped label (case A: case B:) carries no body to break out of.
			}
			innerSwitch, ok := ci.Body[len(ci.Body)-1].(*statements.SwitchStatement)
			if !ok || !switchCompletesNormally(innerSwitch) {
				continue
			}
			fallsThrough := false
			for _, ex := range caseExitsMap[ci] {
				if _, isCaseStart := caseStartNodesMap[ex]; isCaseStart {
					fallsThrough = true
					break
				}
			}
			if fallsThrough {
				continue
			}
			ci.Body = append(ci.Body, statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return "break"
			}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
			}))
		}
	}

	endNodes = utils.NodeFilter(endNodes, func(node *core.Node) bool {
		if slices.Contains(bodyNodes, node) {
			return false
		}
		return !IsEndNode(node)
	})
	for _, node := range NodeDeduplication(endNodes) {
		switchNode.AddNext(node)
	}

	return nil
}
