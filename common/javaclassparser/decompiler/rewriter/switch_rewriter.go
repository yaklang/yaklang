package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"golang.org/x/exp/maps"
	"math"
	"sort"
)

func SwitchRewriter(manager *StatementManager) error {
	for _, node := range manager.SwitchNode {
		err := rewriteSwitch(node, manager)
		if err != nil {
			return err
		}
	}
	return nil
}

func rewriteSwitch(node *core.Node, manager *StatementManager) error {
	switchNode := node
	middleStatement := node.Statement.(*statements.MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseMap := switchData[0].(map[int]*core.Node)
	data := switchData[1].(values.JavaValue)
	//defaultCase := caseMap[-1]
	//delete(caseMap, -1)
	//_ = defaultCase
	caseMapKeys := maps.Keys(caseMap)
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
	node.Statement = switchStatement
	node.RemoveAllNext()
	caseStartNodesMap := map[*core.Node]struct{}{}
	for _, startNode := range caseMap {
		caseStartNodesMap[startNode] = struct{}{}
	}

	keyMap := maps.Keys(caseMap)
	sort.Ints(keyMap)
	keyMap = append(keyMap[1:], -1)
	var hasBreak bool
	for _, v := range keyMap {
		startNode := caseMap[v]
		err := core.WalkGraph[*core.Node](startNode, func(node *core.Node) ([]*core.Node, error) {
			if hasBreak {
				return nil, nil
			}
			if _, ok := caseStartNodesMap[node]; ok && node != startNode {
				return nil, nil
			}
			if v != -1 && switchNode.SwitchMergeNode == node {
				hasBreak = true
				return nil, nil
			}
			return node.Next, nil
		})
		if err != nil {
			return err
		}
	}
	if hasBreak {
		node.AddNext(node.SwitchMergeNode)
	}
	manager.AddFinalAction(func() error {
		for _, v := range keyMap {
			startNode := caseMap[v]
			caseItem := statements.NewCaseItem(v, nil)
			caseItem.IsDefault = v == -1
			if caseItem.IsDefault {
				caseItem.IntValue = math.MaxInt
			}
			var terminalIsEndNode bool
			elements, err := manager.ToStatementsFromNode(startNode, func(node *core.Node) bool {
				if _, ok := caseStartNodesMap[node]; ok && node != startNode {
					return false
				}
				if !caseItem.IsDefault && switchNode.SwitchMergeNode == node {
					terminalIsEndNode = true
					return false
				}
				if caseItem.IsDefault && hasBreak && node == switchNode.SwitchMergeNode {
					terminalIsEndNode = true
					return false
				}
				return true
			})
			if err != nil {
				return err
			}
			sts := core.NodesToStatements(elements)

			if terminalIsEndNode && hasBreak && !caseItem.IsDefault {
				sts = append(sts, statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
					return "break"
				}))
			}
			if caseItem.IsDefault && len(sts) == 2 {
				print()
			}
			caseItem.Body = sts
			caseItems = append(caseItems, caseItem)
		}
		switchStatement.Cases = caseItems
		sort.Slice(caseItems, func(i, j int) bool {
			return caseItems[i].IntValue < caseItems[j].IntValue
		})
		return nil
	})
	return nil
	//for i, key := range caseMapKeys {
	//	i := i
	//	key := key
	//	caseNode := manager.GetNodeById(caseMap[key])
	//	getNextNode := func() *core.Node {
	//		if i == len(caseMapKeys)-1 {
	//			return manager.GetNodeById(defaultCase)
	//		}
	//		return manager.GetNodeById(caseMap[caseMapKeys[i+1]])
	//	}
	//	parseCaseBody := func() (*statements.CaseItem, bool) {
	//		bodyStatements := []statements.Statement{}
	//		var hasBreak bool
	//		caseManager := NewStatementManager(caseNode, manager)
	//		//var preNode *core.Node
	//		err := caseManager.Rewrite()
	//		if err != nil {
	//			return nil, false
	//		}
	//		resStats, err := caseManager.ToStatements(func(node *core.Node) bool {
	//			for _, nextNode := range node.Next {
	//				ok := func() bool {
	//					if nextNode == getNextNode() {
	//						return false
	//					}
	//					if nextNode.Id > getNextNode().Id {
	//						hasBreak = true
	//						breakNode[key] = nextNode
	//						return false
	//					}
	//					return true
	//				}()
	//				if ok {
	//					return true
	//				}
	//			}
	//			return false
	//		})
	//		if err != nil {
	//			return nil, false
	//		}
	//		bodyStatements = core.NodesToStatements(resStats)
	//		item := statements.NewCaseItem(key, bodyStatements)
	//		if hasBreak {
	//			//replaceBreakCB = append(replaceBreakCB, func() {
	//			//	if len(item.Body) > 0 {
	//			//		item.Body = append(item.Body, NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
	//			//			return "break"
	//			//		}))
	//			//	}
	//			//})
	//		}
	//		return item, true
	//	}
	//	if i == 0 {
	//		statementPatternCheck = append(statementPatternCheck, func() bool {
	//			if len(caseNode.Source) != 1 {
	//				return false
	//			}
	//			if caseNode.Source[0] != node {
	//				return false
	//			}
	//			return true
	//		})
	//
	//	} else {
	//		statementPatternCheck = append(statementPatternCheck, func() bool {
	//			if i != 0 && breakNode[caseMapKeys[i-1]] != nil {
	//				if len(caseNode.Source) != 1 {
	//					return false
	//				}
	//			} else {
	//				if len(caseNode.Source) != 2 {
	//					return false
	//				}
	//			}
	//			if caseNode.Source[0] != node {
	//				return false
	//			}
	//			return true
	//		})
	//	}
	//	item, ok := parseCaseBody()
	//	if !ok {
	//		return
	//	}
	//	caseItems = append(caseItems, item)
	//}
	//for _, f := range statementPatternCheck {
	//	if !f() {
	//		return
	//	}
	//}
	//var preNode *core.Node
	//if len(breakNode) > 0 {
	//	for _, n := range breakNode {
	//		if preNode == nil {
	//			preNode = n
	//		} else {
	//			if n != preNode {
	//				return
	//			}
	//		}
	//	}
	//}
	//for _, f := range replaceBreakCB {
	//	f()
	//}
	//newBreakStatement := func() statements.Statement {
	//	return statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
	//		return "break"
	//	})
	//}
	//if preNode != nil {
	//	switchStatement := statements.NewSwitchStatement(data, caseItems)
	//	node.Statement = switchStatement
	//	preNode.Source = []*core.Node{node}
	//	node.Next = []*core.Node{preNode}
	//	defaultCaseNode := statements.NewCaseItem(-1, []statements.Statement{})
	//	defaultCaseNode.IsDefault = true
	//	defaultCaseNodeStart := manager.GetNodeById(defaultCase)
	//	if defaultCaseNodeStart == preNode {
	//		core.VisitBody(switchStatement, func(statement statements.Statement) statements.Statement {
	//			if gotoStat, ok := statement.(*statements.GOTOStatement); ok {
	//				if gotoStat.ToStatement == preNode.Id {
	//					return newBreakStatement()
	//				}
	//			}
	//			return statement
	//		})
	//		return
	//	}
	//	defaultManager := NewStatementManager(defaultCaseNodeStart, manager)
	//	err := defaultManager.Rewrite()
	//	if err != nil {
	//		return
	//	}
	//	defaultBodySts, err := defaultManager.ToStatements(func(node *core.Node) bool {
	//		if node.Next[0] == preNode {
	//			return false
	//		}
	//		return true
	//	})
	//	if err != nil {
	//		return
	//	}
	//	defaultCaseNode.Body = core.NodesToStatements(defaultBodySts)
	//	switchStatement.Cases = append(switchStatement.Cases, defaultCaseNode)
	//	core.VisitBody(switchStatement, func(statement statements.Statement) statements.Statement {
	//		if gotoStat, ok := statement.(*statements.GOTOStatement); ok {
	//			if gotoStat.ToStatement == preNode.Id {
	//				return newBreakStatement()
	//			}
	//		}
	//		return statement
	//	})
	//} else {
	//	switchStatement := statements.NewSwitchStatement(data, caseItems)
	//	node.Statement = switchStatement
	//	node.Next = nil
	//	defaultCaseNode := statements.NewCaseItem(-1, []statements.Statement{})
	//	defaultCaseNode.IsDefault = true
	//	defaultManager := NewStatementManager(manager.GetNodeById(defaultCase), manager)
	//	err := defaultManager.Rewrite()
	//	if err != nil {
	//		return
	//	}
	//	sts, err := defaultManager.ToStatements(func(node *core.Node) bool {
	//		return true
	//	})
	//	if err != nil {
	//		return
	//	}
	//	defaultCaseNode.Body = core.NodesToStatements(sts)
	//	switchStatement.Cases = append(switchStatement.Cases, defaultCaseNode)
	//	//VisitBody(switchStatement, func(statement Statement) Statement {
	//	//	if gotoStat, ok := statement.(*GOTOStatement); ok {
	//	//		if gotoStat.ToStatement == preNode.Id {
	//	//			return newBreakStatement()
	//	//		}
	//	//	}
	//	//	return statement
	//	//})
	//}
}
