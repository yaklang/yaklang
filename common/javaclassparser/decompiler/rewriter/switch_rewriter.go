package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"golang.org/x/exp/maps"
	"sort"
)

func SwitchRewriter(manager *StatementManager, node *core.Node) error {
	if v, ok := node.Statement.(*core.MiddleStatement); ok && v.Flag == core.MiddleSwitch {
		rewriteSwitch(node, manager)
	}
	return nil
}

func rewriteSwitch(node *core.Node, manager *StatementManager) {
	middleStatement := node.Statement.(*core.MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseMap := switchData[0].(map[int]int)
	data := switchData[1].(core.JavaValue)
	defaultCase := caseMap[-1]
	delete(caseMap, -1)
	_ = defaultCase
	caseMapKeys := maps.Keys(caseMap)
	sort.Ints(caseMapKeys)
	caseItems := []*core.CaseItem{}
	// case start node source must content switch node
	breakNode := map[int]*core.Node{}
	replaceBreakCB := []func(){}
	statementPatternCheck := []func() bool{}
	for i, key := range caseMapKeys {
		i := i
		key := key
		caseNode := manager.GetNodeById(caseMap[key])
		getNextNode := func() *core.Node {
			if i == len(caseMapKeys)-1 {
				return manager.GetNodeById(defaultCase)
			}
			return manager.GetNodeById(caseMap[caseMapKeys[i+1]])
		}
		parseCaseBody := func() (*core.CaseItem, bool) {
			bodyStatements := []core.Statement{}
			var hasBreak bool
			caseManager := NewStatementManager(caseNode, manager)
			//var preNode *core.Node
			err := caseManager.Rewrite()
			if err != nil {
				return nil, false
			}
			resStats, err := caseManager.ToStatements(func(node *core.Node) bool {
				for _, nextNode := range node.Next {
					ok := func() bool {
						if nextNode == getNextNode() {
							return false
						}
						if nextNode.Id > getNextNode().Id {
							hasBreak = true
							breakNode[key] = nextNode
							return false
						}
						return true
					}()
					if ok {
						return true
					}
				}
				return false
			})
			if err != nil {
				return nil, false
			}
			bodyStatements = utils.NodesToStatements(resStats)
			item := core.NewCaseItem(key, bodyStatements)
			if hasBreak {
				//replaceBreakCB = append(replaceBreakCB, func() {
				//	if len(item.Body) > 0 {
				//		item.Body = append(item.Body, NewCustomStatement(func(funcCtx *FunctionContext) string {
				//			return "break"
				//		}))
				//	}
				//})
			}
			return item, true
		}
		if i == 0 {
			statementPatternCheck = append(statementPatternCheck, func() bool {
				if len(caseNode.Source) != 1 {
					return false
				}
				if caseNode.Source[0] != node {
					return false
				}
				return true
			})

		} else {
			statementPatternCheck = append(statementPatternCheck, func() bool {
				if i != 0 && breakNode[caseMapKeys[i-1]] != nil {
					if len(caseNode.Source) != 1 {
						return false
					}
				} else {
					if len(caseNode.Source) != 2 {
						return false
					}
				}
				if caseNode.Source[0] != node {
					return false
				}
				return true
			})
		}
		item, ok := parseCaseBody()
		if !ok {
			return
		}
		caseItems = append(caseItems, item)
	}
	for _, f := range statementPatternCheck {
		if !f() {
			return
		}
	}
	var preNode *core.Node
	if len(breakNode) > 0 {
		for _, n := range breakNode {
			if preNode == nil {
				preNode = n
			} else {
				if n != preNode {
					return
				}
			}
		}
	}
	for _, f := range replaceBreakCB {
		f()
	}
	newBreakStatement := func() core.Statement {
		return core.NewCustomStatement(func(funcCtx *core.FunctionContext) string {
			return "break"
		})
	}
	if preNode != nil {
		switchStatement := core.NewSwitchStatement(data, caseItems)
		node.Statement = switchStatement
		preNode.Source = []*core.Node{node}
		node.Next = []*core.Node{preNode}
		defaultCaseNode := core.NewCaseItem(-1, []core.Statement{})
		defaultCaseNode.IsDefault = true
		defaultCaseNodeStart := manager.GetNodeById(defaultCase)
		if defaultCaseNodeStart == preNode {
			core.VisitBody(switchStatement, func(statement core.Statement) core.Statement {
				if gotoStat, ok := statement.(*core.GOTOStatement); ok {
					if gotoStat.ToStatement == preNode.Id {
						return newBreakStatement()
					}
				}
				return statement
			})
			return
		}
		defaultManager := NewStatementManager(defaultCaseNodeStart, manager)
		err := defaultManager.Rewrite()
		if err != nil {
			return
		}
		defaultBodySts, err := defaultManager.ToStatements(func(node *core.Node) bool {
			if node.Next[0] == preNode {
				return false
			}
			return true
		})
		if err != nil {
			return
		}
		defaultCaseNode.Body = utils.NodesToStatements(defaultBodySts)
		switchStatement.Cases = append(switchStatement.Cases, defaultCaseNode)
		core.VisitBody(switchStatement, func(statement core.Statement) core.Statement {
			if gotoStat, ok := statement.(*core.GOTOStatement); ok {
				if gotoStat.ToStatement == preNode.Id {
					return newBreakStatement()
				}
			}
			return statement
		})
	} else {
		switchStatement := core.NewSwitchStatement(data, caseItems)
		node.Statement = switchStatement
		node.Next = nil
		defaultCaseNode := core.NewCaseItem(-1, []core.Statement{})
		defaultCaseNode.IsDefault = true
		defaultManager := NewStatementManager(manager.GetNodeById(defaultCase), manager)
		err := defaultManager.Rewrite()
		if err != nil {
			return
		}
		sts, err := defaultManager.ToStatements(func(node *core.Node) bool {
			return true
		})
		if err != nil {
			return
		}
		defaultCaseNode.Body = utils.NodesToStatements(sts)
		switchStatement.Cases = append(switchStatement.Cases, defaultCaseNode)
		//VisitBody(switchStatement, func(statement Statement) Statement {
		//	if gotoStat, ok := statement.(*GOTOStatement); ok {
		//		if gotoStat.ToStatement == preNode.Id {
		//			return newBreakStatement()
		//		}
		//	}
		//	return statement
		//})
	}
}

//func ScanSingleRoute(node *core.Node, hanlde func(node2 *core.Node) bool) {
//	if node == nil {
//		return
//	}
//	if !hanlde(node) {
//		return
//	}
//	if len(node.Next) != 1 {
//		return
//	}
//	for _, next := range node.Next {
//		ScanSingleRoute(next, hanlde)
//	}
//}
//func CheckSourceNode(node *core.Node, source []*core.Node) bool {
//	if node == nil {
//		return false
//	}
//	if len(node.Source) != len(source) {
//		return false
//	}
//	for i, v := range source {
//		if node.Source[i] != v {
//			return false
//		}
//	}
//	return true
//}
//
////func CheckSingleRouteToNode(start, end *core.Node) bool {
////	if node == nil {
////		return false
////	}
////	if len(node.Next) == 0 {
////		return true
////	}
////	if len(node.Next) > 1 {
////		return false
////	}
////	return CheckSingleRouteToNode(node.Next[0])
////
////}
