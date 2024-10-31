package decompiler

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/rewriter"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestSwitch(t *testing.T) {
	id := 0
	newSwitch := func(startNodes []*core.Node) *core.Node {
		m := map[int]*core.Node{}
		for i, node := range startNodes {
			if i == len(startNodes)-1 {
				m[-1] = node
				continue
			}
			m[i] = node
		}
		node := core.NewNode(statements.NewMiddleStatement(statements.MiddleSwitch, []any{m, values.NewJavaLiteral("switch", types.NewJavaPrimer(types.JavaString))}))
		node.Id = id
		id++
		for _, n := range startNodes {
			node.AddNext(n)
		}
		return node
	}
	newCommonNode := func(name string) *core.Node {
		node := core.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
			return name
		}))
		node.Id = id
		id++
		return node
	}
	case1 := newCommonNode("case1 body")
	case2 := newCommonNode("case2 body")
	case3 := newCommonNode("case3 body")
	defaultNode := newCommonNode("default body")
	switchNode := newSwitch([]*core.Node{case1, case2, case3, defaultNode})
	endNode := newCommonNode("end")
	case1.AddNext(case2)
	case2.AddNext(case3)
	case3.AddNext(defaultNode)
	defaultNode.AddNext(endNode)
	//rewriter.GenerateDominatorTree(switchNode)
	println(utils.DumpNodesToDotExp(switchNode))

	statementManager := rewriter.NewRootStatementManager(switchNode)
	statementManager.SetId(id)
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
	//println(utils.DumpNodesToDotExp(start))
	assert.Equal(t, 1, len(statementManager.SwitchNode), "switch nodes")
	assert.Equal(t, switchNode, statementManager.SwitchNode[0], "switch node")
	//assert.Equal(t, endNode, statementManager.SwitchNode[0].SwitchMergeNode, "switch merge node")
	err = statementManager.Rewrite()
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
	assert.Equal(t, `switch("switch") {
case 0:
case1 body
case 1:
case2 body
case 2:
case3 body
default:
default bodyend
}`, strings.Join(statementsStrs, "\n"))
}

func TestSwitch2(t *testing.T) {
	id := 0
	newSwitch := func(startNodes []*core.Node) *core.Node {
		m := map[int]*core.Node{}
		for i, node := range startNodes {
			if i == len(startNodes)-1 {
				m[-1] = node
				continue
			}
			m[i] = node
		}
		node := core.NewNode(statements.NewMiddleStatement(statements.MiddleSwitch, []any{m, values.NewJavaLiteral("switch", types.NewJavaPrimer(types.JavaString))}))
		node.Id = id
		id++
		for _, n := range startNodes {
			node.AddNext(n)
		}
		return node
	}
	newCommonNode := func(name string) *core.Node {
		node := core.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
			return name
		}))
		node.Id = id
		id++
		return node
	}
	case1 := newCommonNode("case1 body")
	case2 := newCommonNode("case2 body")
	case3 := newCommonNode("case3 body")
	defaultNode := newCommonNode("default body")
	switchNode := newSwitch([]*core.Node{case1, case2, case3, defaultNode})
	endNode := newCommonNode("end")
	case1.AddNext(case2)
	case2.AddNext(endNode)
	case3.AddNext(defaultNode)
	defaultNode.AddNext(endNode)
	//rewriter.GenerateDominatorTree(switchNode)
	println(utils.DumpNodesToDotExp(switchNode))

	statementManager := rewriter.NewRootStatementManager(switchNode)
	statementManager.SetId(id)
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
	//println(utils.DumpNodesToDotExp(start))
	assert.Equal(t, 1, len(statementManager.SwitchNode), "switch nodes")
	assert.Equal(t, switchNode, statementManager.SwitchNode[0], "switch node")
	assert.Equal(t, endNode, statementManager.SwitchNode[0].SwitchMergeNode, "switch merge node")
	err = statementManager.Rewrite()
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
	assert.Equal(t, `switch("switch") {
case 0:
case1 body
case 1:
case2 bodybreak
case 2:
case3 body
default:
default body
}
end`, strings.Join(statementsStrs, "\n"))
}
