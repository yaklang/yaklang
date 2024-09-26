package decompiler

import (
	"golang.org/x/exp/maps"
	"sort"
)

func SwitchRewriter(manager *StatementManager, node *Node) error {
	if v, ok := node.Statement.(*MiddleStatement); ok && v.Flag == MiddleSwitch {
		rewriteSwitch(node, manager)
	}
	return nil
}

func rewriteSwitch(node *Node, manager *StatementManager) {
	middleStatement := node.Statement.(*MiddleStatement)
	switchData := middleStatement.Data.([]any)
	caseMap := switchData[0].(map[int]int)
	data := switchData[1].(JavaValue)
	defaultCase := caseMap[-1]
	delete(caseMap, -1)
	_ = defaultCase
	caseMapKeys := maps.Keys(caseMap)
	sort.Ints(caseMapKeys)
	caseItems := []*CaseItem{}
	// case start node source must content switch node
	breakNode := map[int]*Node{}
	replaceBreakCB := []func(){}
	statementPatternCheck := []func() bool{}
	for i, key := range caseMapKeys {
		i := i
		key := key
		caseNode := manager.GetNodeById(caseMap[key])
		getNextNode := func() *Node {
			if i == len(caseMapKeys)-1 {
				return manager.GetNodeById(defaultCase)
			}
			return manager.GetNodeById(caseMap[caseMapKeys[i+1]])
		}
		parseCaseBody := func() (*CaseItem, bool) {
			bodyStatements := []Statement{}
			var hasBreak bool
			caseManager := NewStatementManager(caseNode)
			//var preNode *Node
			err := caseManager.Rewrite(func(node *Node) bool {
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
			resStats, err := caseManager.ToStatements()
			if err != nil {
				return nil, false
			}
			bodyStatements = resStats
			item := NewCaseItem(key, bodyStatements)
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
	var preNode *Node
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
	newBreakStatement := func() Statement {
		return NewCustomStatement(func(funcCtx *FunctionContext) string {
			return "break"
		})
	}
	if preNode != nil {
		switchStatement := NewSwitchStatement(data, caseItems)
		node.Statement = switchStatement
		preNode.Source = []*Node{node}
		node.Next = []*Node{preNode}
		defaultCaseNode := NewCaseItem(-1, []Statement{})
		defaultCaseNode.IsDefault = true
		defaultCaseNodeStart := manager.GetNodeById(defaultCase)
		if defaultCaseNodeStart == preNode {
			VisitBody(switchStatement, func(statement Statement) Statement {
				if gotoStat, ok := statement.(*GOTOStatement); ok {
					if gotoStat.ToStatement == preNode.Id {
						return newBreakStatement()
					}
				}
				return statement
			})
			return
		}
		defaultManager := NewStatementManager(defaultCaseNodeStart)
		err := defaultManager.Rewrite(func(node *Node) bool {
			if node.Next[0] == preNode {
				return false
			}
			return true
		})
		if err != nil {
			return
		}
		defaultBodySts, err := defaultManager.ToStatements()
		if err != nil {
			return
		}
		defaultCaseNode.Body = defaultBodySts
		switchStatement.Cases = append(switchStatement.Cases, defaultCaseNode)
		VisitBody(switchStatement, func(statement Statement) Statement {
			if gotoStat, ok := statement.(*GOTOStatement); ok {
				if gotoStat.ToStatement == preNode.Id {
					return newBreakStatement()
				}
			}
			return statement
		})
	} else {
		switchStatement := NewSwitchStatement(data, caseItems)
		node.Statement = switchStatement
		node.Next = nil
		defaultCaseNode := NewCaseItem(-1, []Statement{})
		defaultCaseNode.IsDefault = true
		defaultManager := NewStatementManager(manager.GetNodeById(defaultCase))
		err := defaultManager.Rewrite(func(node *Node) bool {
			return true
		})
		if err != nil {
			return
		}
		sts, err := defaultManager.ToStatements()
		if err != nil {
			return
		}
		defaultCaseNode.Body = sts
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

//func ScanSingleRoute(node *Node, hanlde func(node2 *Node) bool) {
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
//func CheckSourceNode(node *Node, source []*Node) bool {
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
////func CheckSingleRouteToNode(start, end *Node) bool {
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
