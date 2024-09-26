package decompiler

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

type Rewriter func(statementManager *StatementManager, node *Node) error

func RewriteIf(statementManager *StatementManager, node *Node) error {
	ifNode := node
	if _, ok := ifNode.Statement.(*ConditionStatement); !ok {
		return nil
	}
	if len(ifNode.Next) != 2 {
		return nil
	}
	ifBodyStartNode := ifNode.Next[1]
	elseBodyStartNode := ifNode.Next[0]

	// search merge node, if founded successful, conform that this is an if statement
	ifBodyManager := NewStatementManager(ifBodyStartNode)
	ifNodeRecord := utils.NewSet[*Node]()
	ifBodyManager.ScanStatementSimple(func(node *Node) error {
		ifNodeRecord.Add(node)
		return nil
	})
	var mergeNode *Node
	elseBodyManager := NewStatementManager(elseBodyStartNode)
	elseNodeRecord := utils.NewSet[*Node]()
	elseBodyManager.ScanStatement(func(node *Node) (error, bool) {
		if ifNodeRecord.Has(node) {
			mergeNode = node
			return nil, false
		}
		elseNodeRecord.Add(node)
		return nil, true
	})
	if mergeNode == nil {
		return nil
	}
	//
	//ifNodeRecord = utils.NewSet[*Node]()
	//ifBodyManager.ScanStatement(func(node *Node) (error, bool) {
	//	if node == mergeNode {
	//		return nil, false
	//	}
	//	ifNodeRecord.Add(node)
	//	return nil, true
	//})
	//var elseEndNode *Node
	//
	//if ifNodeRecord.Has(elseBodyStartNode) {
	//	mergeNode = elseBodyStartNode
	//	elseEndNode = elseBodyStartNode
	//} else {
	//	elseBodyManager.ScanStatement(func(node *Node) (error, bool) {
	//		for _, n := range node.Next {
	//			if ifNodeRecord.Has(n) {
	//				mergeNode = n
	//				elseEndNode = node
	//				return nil, false
	//			}
	//		}
	//		return nil, true
	//	})
	//}
	//if mergeNode == nil {
	//	return nil
	//}
	//if len(mergeNode.Source) < 2 {
	//	return nil
	//}
	//mergeNodeSource := mergeNode.Source
	//// cutlink
	//for _, n := range mergeNodeSource {
	//	if n == elseEndNode || ifNodeRecord.Has(n) {
	//		CutNode(n, mergeNode)
	//	}
	//}
	var ifBodyStatements, elseBodyStatements []Statement
	if ifBodyStartNode != mergeNode {
		err := ifBodyManager.Rewrite(func(node *Node) bool {
			if len(node.Next) == 1 && node.Next[0] == mergeNode {
				return false
			}
			return true
		})
		if err != nil {
			return nil
		}
		ifBody, err := ifBodyManager.ToStatements()
		if err != nil {
			return nil
		}
		ifBodyStatements = ifBody
	}
	if elseBodyStartNode != mergeNode {
		err := elseBodyManager.Rewrite(func(node *Node) bool {
			if len(node.Next) == 0 && node.Next[0] == mergeNode {
				return false
			}
			return true
		})
		if err != nil {
			return nil
		}
		elseBody, err := elseBodyManager.ToStatements()
		if err != nil {
			return nil
		}
		elseBodyStatements = elseBody
	}
	ifStatement := NewIfStatement(ifNode.Statement.(*ConditionStatement).Condition, ifBodyStatements, elseBodyStatements)
	ifStatementNode := NewNode(ifStatement)
	//statementManager.DeleteStatementById(ifNode.Id)
	ifStatementNode.Id = ifNode.Id
	ifStatementNode.Source = node.Source
	for _, n := range ifNode.Source {
		for i, nextNode := range n.Next {
			if nextNode.Id == ifNode.Id {
				n.Next[i] = ifStatementNode
			}
		}
	}
	mergeNode.Source = append(mergeNode.Source, ifStatementNode)
	ifStatementNode.Next = []*Node{mergeNode}
	*node = *ifStatementNode
	return nil
}

//func _RewriteIf(statementManager *StatementManager) error {
//	nodes := statementManager.GetNodes()
//	entryPoint := []int{}
//	for i, node := range nodes {
//		if _, ok := node.Statement.(*ConditionStatement); ok {
//			entryPoint = append(entryPoint, i)
//		}
//	}
//	scanWithRewriter := func(rewriter func(nodes []*Node, index int)) {
//		for _, i := range entryPoint {
//			if nodes[i] == nil {
//				continue
//			}
//			rewriter(nodes, i)
//		}
//	}
//	scanWithRewriter(rewriteTernaryExpression)
//	scanWithRewriter(rewriteLogicalExpressions)
//	for i, _ := range nodes {
//		if nodes[i] == nil {
//			continue
//		}
//		_, ok := nodes[i].Statement.(*ConditionStatement)
//		if !ok {
//			continue
//		}
//		rewriteIf(nodes, i)
//	}
//	newNodes := []*Node{}
//	for _, node := range nodes {
//		if node != nil {
//			newNodes = append(newNodes, node)
//		}
//	}
//	statementManager.SetNodes(newNodes)
//	return nil
//}

// rewriteIf
// if condition goto xxx
//
//	...
//
// goto xxx
//
//	...
//
// ==> if condition {}else{}
func rewriteIf(nodes []*Node, index int) {
	conditionStatement, ok := nodes[index].Statement.(*ConditionStatement)
	if !ok {
		return
	}
	idToIndexMap := map[int]int{}
	for i, node := range nodes {
		if node == nil {
			continue
		}
		idToIndexMap[node.Id] = i
	}
	idToIndex := func(id int) int {
		return idToIndexMap[id]
	}
	ifBodyStart := index + 1
	elseBodyStart := idToIndex(conditionStatement.ToStatement)
	var gotoStatement *GOTOStatement
	gotoStatementIndex := -1
	for i := elseBodyStart - 1; i >= ifBodyStart; i-- {
		if gotoSt, ok := nodes[i].Statement.(*GOTOStatement); ok {
			gotoStatement = gotoSt
			gotoStatementIndex = i
			break
		}
	}
	var ifBodyEnd int
	var elseBodyEnd int
	if gotoStatement == nil { // 不存在else body
		ifBodyEnd = elseBodyStart
		elseBodyEnd = elseBodyStart
	} else {
		ifBodyEnd = gotoStatementIndex
		fmt.Println(gotoStatement.ToStatement)
		elseBodyEnd = idToIndex(gotoStatement.ToStatement)
	}
	fmt.Println(elseBodyEnd)
	fmt.Println(ifBodyStart)
	ifBody := nodes[ifBodyStart:ifBodyEnd]
	elseBody := nodes[elseBodyStart:elseBodyEnd]
	getBody := func(nodes []*Node) []Statement {
		var res []Statement
		for _, node := range nodes {
			res = append(res, node.Statement)
		}
		return res
	}
	ifStatement := NewIfStatement(conditionStatement.Condition, getBody(ifBody), getBody(elseBody))
	nodes[index].Statement = ifStatement
	for i := ifBodyStart; i < elseBodyEnd; i++ {
		nodes[i] = nil
	}
}

// rewriteLogicalExpressions
// var1 = 1 > 2 ? true : false ==> var1 = 1 > 2
func rewriteLogicalExpressions(nodes []*Node, index int) {
	assignStatement, ok := nodes[index].Statement.(*AssignStatement)
	if !ok {
		return
	}
	if assignStatement.JavaValue == nil {
		return
	}
	ternaryExpression, ok := assignStatement.JavaValue.(*TernaryExpression)
	if !ok {
		return
	}
	funCtx := &FunctionContext{}
	if ternaryExpression.Type().String(funCtx) != JavaBoolean.String(funCtx) {
		return
	}
	if ternaryExpression.TrueValue == nil || ternaryExpression.FalseValue == nil {
		return
	}
	if ternaryExpression.TrueValue.Type().String(funCtx) == JavaInteger.String(funCtx) && ternaryExpression.FalseValue.Type().String(funCtx) == JavaInteger.String(funCtx) {
		assignStatement.JavaValue = ternaryExpression.Condition
	}
}

// rewriteTernaryExpression
//
//	if (condition){
//		stack_var1 = var1
//	}else{
//		stack_var1 = var2
//	}
//
// var3 = stack_var1
//
//	>> var3 = condition ? var1 : var2
func rewriteTernaryExpression(nodes []*Node, index int) {
	conditionStatement, ok := nodes[index].Statement.(*ConditionStatement)
	if !ok {
		return
	}
	idToIndexMap := map[int]int{}
	for i, node := range nodes {
		if node == nil {
			continue
		}
		idToIndexMap[node.Id] = i
	}
	idToIndex := func(id int) int {
		return idToIndexMap[id]
	}
	elseStart := nodes[idToIndex(conditionStatement.ToStatement)]
	stackAssign1 := nodes[index+1] // if 语句下是assign1
	stackAssign2 := elseStart      // else 语句下是assign2

	// assign1 和 assign2 都是 stackAssignStatement
	stackAssign1Statement, ok := stackAssign1.Statement.(*StackAssignStatement)
	if !ok {
		return
	}
	stackAssign2Statement, ok := stackAssign2.Statement.(*StackAssignStatement)
	if !ok {
		return
	}
	// id 相同
	if stackAssign1Statement.Id != stackAssign2Statement.Id {
		return
	}

	// assign2下面是赋值,assign1下面是goto,跳转到assign2下面
	gotoSt, ok := nodes[index+2].Statement.(*GOTOStatement)
	if !ok {
		return
	}
	if gotoSt.ToStatement != conditionStatement.ToStatement+1 {
		return
	}
	assignSt, ok := nodes[idToIndex(gotoSt.ToStatement)].Statement.(*AssignStatement)
	if !ok {
		return
	}
	exp := NewTernaryExpression(conditionStatement.Condition, stackAssign1Statement.JavaValue, stackAssign2Statement.JavaValue)
	assignSt.JavaValue = exp
	for i := index; i < index+4; i++ {
		nodes[i] = nil
	}
}

//func scanNodes(nodes []*Node,condition func(node *Node, index int) bool, rewriter Rewriter) {
//	for i, node := range nodes {
//		if node == nil {
//			continue
//		}
//		if condition(node, i){
//			rewriter(nodes, i)
//		}
//	}
//}
