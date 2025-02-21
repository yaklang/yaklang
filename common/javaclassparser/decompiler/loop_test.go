package decompiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils3 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/rewriter"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

type graphBuilder struct {
	id int
}

func newGraphBuilder() *graphBuilder {
	return &graphBuilder{
		id: 0,
	}
}
func (g *graphBuilder) NewNode(name string) *core.Node {
	node := core.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
		return name
	}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
	}))
	node.Id = g.id
	g.id++
	return node
}
func (g *graphBuilder) NewTry(name string) *core.Node {
	node := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(name, types.NewJavaPrimer(types.JavaString)), ""))
	node.Id = g.id
	g.id++
	node.TrueNode = func() *core.Node {
		return node.Next[0]
	}
	node.FalseNode = func() *core.Node {
		return node.Next[1]
	}
	return node
}
func (g *graphBuilder) NewIf(name string) *core.Node {
	node := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(name, types.NewJavaPrimer(types.JavaString)), ""))
	node.Id = g.id
	g.id++
	node.TrueNode = func() *core.Node {
		return node.Next[0]
	}
	node.FalseNode = func() *core.Node {
		return node.Next[1]
	}
	return node
}

func dumpGraph(node *core.Node) (string, error) {
	rewriter := rewriter.NewRootStatementManager(node)
	err := rewriter.Rewrite()
	if err != nil {
		return "", err
	}
	sts, err := rewriter.ToStatements(nil)
	if err != nil {
		return "", err
	}
	statementsStrs := []string{}
	sts1 := core.NodesToStatements(sts)
	for _, st := range sts1 {
		statementsStrs = append(statementsStrs, st.String(&class_context.ClassContext{}))
	}
	return strings.Join(statementsStrs, "\n"), nil
}

// TestLoopDoWhile the do while loop has many break and continue statements
func TestLoopDoWhile(t *testing.T) {
	builder := newGraphBuilder()
	newIf := builder.NewIf
	newCommonNode := builder.NewNode
	start := newCommonNode("start")
	ifOther := newIf("if other")
	ifOtherBody := newCommonNode("if other body")
	loopStart := newCommonNode("loop start")
	loopCondition := newIf("while condition")
	bodyIf1 := newIf("if1")
	if2 := newIf("if2")
	if1Body := newCommonNode("if1 body")
	if4 := newIf("if4")
	bodyIf3 := newIf("if3")
	if3Body := newCommonNode("if3 body")
	loopEnd := newCommonNode("while end")

	loopStart.AddNext(loopCondition)
	ifOther.AddNext(loopStart)
	ifOther.AddNext(ifOtherBody)
	ifOtherBody.AddNext(loopEnd)
	start.AddNext(ifOther)
	loopCondition.AddNext(bodyIf1)
	bodyIf1.AddNext(if2)
	if1Body.AddNext(loopStart)
	bodyIf1.AddNext(if1Body)
	if4.AddNext(loopEnd)
	if4.AddNext(loopStart)
	if2.AddNext(if4)
	if2.AddNext(bodyIf3)
	if3Body.AddNext(loopEnd)
	bodyIf3.AddNext(if3Body)
	bodyIf3.AddNext(loopEnd)
	loopCondition.AddNext(loopEnd)

	println(utils.DumpNodesToDotExp(start))
	statementManager := rewriter.NewRootStatementManager(start)
	statementManager.SetId(builder.id)
	err := statementManager.ScanCoreInfo()
	if err != nil {
		t.Fatal(err)
	}
	compareNodeList := func(nodes1, nodes2 []*core.Node) bool {
		set1 := utils2.NewSet[*core.Node]()
		set1.AddList(nodes1)
		set2 := utils2.NewSet[*core.Node]()
		set2.AddList(nodes2)
		if set1.Diff(set2).Len() == 0 {
			return true
		} else {
			return false
		}
	}
	_ = compareNodeList
	println(utils.DumpNodesToDotExp(start))
	//assert.Equal(t, 6, len(statementManager.IfNodes), "if nodes")
	//assert.Equal(t, 1, len(statementManager.CircleEntryPoint), "circle entry point")
	//assert.Equal(t, loopStart, statementManager.CircleEntryPoint[0], "circle entry point address")
	//assert.Equal(t, loopEnd, loopStart.GetLoopEndNode(), "loop end node")
	//assert.Equal(t, 6, loopStart.CircleNodesSet.Len(), "node in circle set size")
	//assert.Equal(t, true, compareNodeList(loopStart.ConditionNode, []*core.Node{if2, if4, loopCondition}), "node in circle set size")

	sourceCode, err := dumpGraph(start)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `start
if (if other){
if other body
}else{
do{
loop startif (while condition){
if (if1){
if (if2){
if (if4){
break
}else{
continue
}
}else{
if (if3){
if3 body
break
}else{
break
}
}
}else{
if1 body
continue
}
}else{
break
}
}while(true)
}
while end`, sourceCode)
	//println(strings.Join(statementsStrs, "\n"))
}
func TestNestedLoop(t *testing.T) {
	id := 0
	newIf := func(name string) *core.Node {
		node := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(name, types.NewJavaPrimer(types.JavaString)), ""))
		node.Id = id
		id++
		node.TrueNode = func() *core.Node {
			return node.Next[0]
		}
		node.FalseNode = func() *core.Node {
			return node.Next[1]
		}
		return node
	}
	newCommonNode := func(name string) *core.Node {
		node := core.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
			return name
		}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
		}))
		node.Id = id
		id++
		return node
	}
	startNode := newCommonNode("start")
	loop1 := newIf("loop1 start")
	loop1Body := newCommonNode("loop1 body")
	loop2 := newIf("loop2 start")
	loop2Body := newCommonNode("loop2 body")
	loop1End := newCommonNode("loop1 end")

	startNode.AddNext(loop1)
	//loop1.AddNext(loop1Body)
	loop1Body.AddNext(loop1)
	loop1.AddNext(loop2)
	loop2.AddNext(loop2Body)
	loop2Body.AddNext(loop2)
	loop2.AddNext(loop1)
	loop1.AddNext(loop1End)
	println(utils.DumpNodesToDotExp(startNode))
	statementManager := rewriter.NewRootStatementManager(startNode)
	statementManager.SetId(id)
	err := statementManager.Rewrite()
	if err != nil {
		t.Fatal(err)
	}
	sts, err := statementManager.ToStatements(func(node *core.Node) bool {
		return true
	})
	sts = funk.Filter(sts, func(item *core.Node) bool {
		if v, ok := item.Statement.(*statements.CustomStatement); ok {
			if v.Name == "end" {
				return false
			}
		}
		_, ok := item.Statement.(*statements.StackAssignStatement)
		return !ok
	}).([]*core.Node)
	if err != nil {
		t.Fatal(err)
	}
	statementsStrs := []string{}
	for _, st := range core.NodesToStatements(sts) {
		statementsStrs = append(statementsStrs, st.String(&class_context.ClassContext{}))
	}
	println(strings.Join(statementsStrs, "\n"))
	assert.Equal(t, `start
LOOP_1: do{
if (loop1 start){
do{
if (loop2 start){
loop2 body
continue
}else{
continue LOOP_1
}
}while(true)
continue
}else{
break
}
}while(true)
loop1 end`, strings.Join(statementsStrs, "\n"))
}
func TestBreakInLoop(t *testing.T) {
	id := 0
	newIf := func(name string) *core.Node {
		node := core.NewNode(statements.NewConditionStatement(values.NewJavaLiteral(name, types.NewJavaPrimer(types.JavaString)), ""))
		node.Id = id
		id++
		node.TrueNode = func() *core.Node {
			return node.Next[0]
		}
		node.FalseNode = func() *core.Node {
			return node.Next[1]
		}
		return node
	}
	newCommonNode := func(name string) *core.Node {
		node := core.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
			return name
		}, func(oldId *utils3.VariableId, newId *utils3.VariableId) {
		}))
		node.Id = id
		id++
		return node
	}
	startNode := newCommonNode("start")
	loop1 := newIf("loop1 start")
	loop1Body := newCommonNode("loop1 body")
	loop1End := newCommonNode("loop1 end")
	if1 := newIf("if1")

	startNode.AddNext(loop1)
	loop1.AddNext(loop1Body)
	loop1.AddNext(loop1End)
	loop1Body.AddNext(if1)
	if1.AddNext(loop1End)
	if1.AddNext(loop1)
	println(utils.DumpNodesToDotExp(startNode))
	statementManager := rewriter.NewRootStatementManager(startNode)
	statementManager.SetId(id)
	err := statementManager.Rewrite()
	if err != nil {
		t.Fatal(err)
	}
	sts, err := statementManager.ToStatements(func(node *core.Node) bool {
		return true
	})
	sts = funk.Filter(sts, func(item *core.Node) bool {
		if v, ok := item.Statement.(*statements.CustomStatement); ok {
			if v.Name == "end" {
				return false
			}
		}
		_, ok := item.Statement.(*statements.StackAssignStatement)
		return !ok
	}).([]*core.Node)
	if err != nil {
		t.Fatal(err)
	}
	statementsStrs := []string{}
	for _, st := range core.NodesToStatements(sts) {
		statementsStrs = append(statementsStrs, st.String(&class_context.ClassContext{}))
	}
	println(strings.Join(statementsStrs, "\n"))
	assert.Equal(t, `start
do{
if (loop1 start){
loop1 body
if (if1){
break
}else{
continue
}
}else{
break
}
}while(true)
loop1 end`, strings.Join(statementsStrs, "\n"))
}
