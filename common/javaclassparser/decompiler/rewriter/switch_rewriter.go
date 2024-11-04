package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"math"
	"slices"
	"sort"
)

func SwitchRewriter1(manager *RewriteManager, node *core.Node) error {
	middleStatement := node.Statement.(*statements.MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseToIndexMap := switchData[0].(map[int]int)
	caseMap := map[int]*core.Node{}
	for k, v := range caseToIndexMap {
		caseMap[k] = node.Next[v]
	}
	keyMap := maps.Keys(caseMap)
	sort.Ints(keyMap)
	keyMap = append(keyMap[1:], -1)
	startNodes := maps.Values(caseMap)
	endNodes := utils.NodeFilter(manager.DominatorMap[node], func(node *core.Node) bool {
		if v, ok := node.Statement.(*statements.MiddleStatement); ok && (v.Flag == "end" || v.Flag == "start") {
			return false
		}
		return !slices.Contains(startNodes, node)
	})
	//node.RemoveAllNext()
	endNodes = utils2.NewSet[*core.Node](endNodes).List()
	if len(endNodes) > 1 {
		panic("invalid switch node")
	}
	var mergeNode *core.Node
	if len(endNodes) == 1 {
		mergeNode = endNodes[0]
		allSources := slices.Clone(mergeNode.Source)
		mergeNode.RemoveAllSource()
		for _, source := range allSources {
			breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return "break"
			}))
			source.AddNext(breakNode)
		}
		node.AddNext(mergeNode)
	}
	node.MergeNode = mergeNode
	return nil
}
func SwitchRewriter(manager *RewriteManager, node *core.Node) error {
	//switchNode := node
	middleStatement := node.Statement.(*statements.MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseToIndexMap := switchData[0].(map[int]int)
	caseMap := map[int]*core.Node{}
	for k, v := range caseToIndexMap {
		caseMap[k] = node.Next[v]
	}
	data := switchData[1].(values.JavaValue)
	//defaultCase := caseMap[-1]
	//delete(caseMap, -1)
	//_ = defaultCase
	caseMapKeys := maps.Keys(caseMap)
	caseMap[math.MaxInt] = caseMap[-1]
	delete(caseMap, -1)
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
	for _, startNode := range caseMap {
		caseStartNodesMap[startNode] = struct{}{}
	}

	keyMap := maps.Keys(caseMap)
	node.RemoveAllNext()
	switchNode := manager.NewNode(switchStatement)
	node.Replace(switchNode)
	if node.MergeNode != nil {
		node.RemoveNext(node.MergeNode)
		switchNode.AddNext(node.MergeNode)
	}
	nodeToVals := map[*core.Node][]int{}
	for k, v := range caseMap {
		nodeToVals[v] = append(nodeToVals[v], k)
	}
	for k, v := range nodeToVals {
		sort.Ints(v)
		nodeToVals[k] = v
		for i, val := range v {
			if i == len(v)-1 {
				break
			}
			caseMap[val] = nil
		}
	}
	sort.Ints(keyMap)
	for _, v := range keyMap {
		startNode := caseMap[v]
		caseItem := statements.NewCaseItem(v, nil)
		if startNode == nil {
			caseItems = append(caseItems, caseItem)
			continue
		}
		caseItem.IsDefault = v == math.MaxInt
		//var terminalIsEndNode bool
		var sts []statements.Statement
		err := core.WalkGraph(startNode, func(node *core.Node) ([]*core.Node, error) {
			sts = append(sts, node.Statement)
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				}
			}
			return next, nil
		})
		if err != nil {
			return err
		}
		caseItem.Body = sts
		caseItems = append(caseItems, caseItem)
	}
	switchStatement.Cases = caseItems
	sort.Slice(caseItems, func(i, j int) bool {
		return caseItems[i].IntValue < caseItems[j].IntValue
	})
	return nil
}
