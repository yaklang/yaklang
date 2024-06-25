package java

import (
	"fmt"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestFilterOpcode(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain(`stream #-> *?{opcode: param}`).Show(false).Len() <= 0 {
			t.Fatal("FilterOpcode not found")
		}

		prog.Show()
		result, err := prog.SyntaxFlowWithError(`
newDocumentBuilder().parse(* #-> *?{opcode: param && .annotation} as $param)

check $param then "dangerous xml doc builder" else "safe xml doc builder";
alert $param;
`)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		if result.GetValues("param").Recursive(func(operator sfvm.ValueOperator) error {
			count++
			return nil
		}) != nil {
			t.Fatal("param not found")
		}
		if count <= 0 {
			t.Fatal("param not found")
		}

		result, err = prog.SyntaxFlowWithError(`
newDocumentBuilder().parse(* #-> *?{opcode: param && .annotation.*Param} as $param)

check $param then "dangerous xml doc builder" else "safe xml doc builder";
alert $param;
`)
		if err != nil {
			t.Fatal(err)
		}
		result.Show()
		dotGraph := ssaapi.SyntaxFlowVariableToValues(result.GetValues("param")).DotGraph()
		fmt.Println(dotGraph)
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
