package decompiler

import "github.com/yaklang/yaklang/common/go-funk"

func RewriteIf(nodes []*Node) ([]*Node, error) {
	scanWithRewriter := func(rewriter func(nodes []*Node, index int)) {
		for index, node := range nodes {
			if node == nil {
				continue
			}
			rewriter(nodes, index)
		}
		nodes = funk.Filter(nodes, func(item *Node) bool { return item != nil }).([]*Node)
	}
	scanWithRewriter(rewriteTernaryExpression)
	scanWithRewriter(rewriteLogicalExpressions)
	scanWithRewriter(rewriteIf)
	return nodes, nil
}

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
	if gotoStatement == nil {
		return
	}
	ifBodyEnd := gotoStatementIndex
	elseBodyEnd := idToIndex(gotoStatement.ToStatement)
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
