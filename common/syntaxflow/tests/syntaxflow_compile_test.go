package syntaxflow

import (
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
)

func TestSyntaxFlowFilter_Search(t *testing.T) {
	for _, i := range []map[string]string{
		{
			"code":    "exec", // Ref
			"keyword": "push$exact",
		},
		{
			"code":    "ex*",
			"keyword": "push$glob",
		},
		{
			"code":    "/abc/",
			"keyword": "push$regex",
		},
	} {
		vm := sfvm.NewSyntaxFlowVirtualMachine()
		vm.Debug()
		frame, err := vm.Compile(i["code"])
		if err != nil {
			t.Fatal(err)
		}
		vm.Show()
		count := 0
		checked := false
		count += len(frame.Codes)
		for _, c := range frame.Codes {
			if strings.Contains(c.String(), i["keyword"]) {
				checked = true
			}
		}
		if !checked {
			t.Fatalf("SyntaxFlowVirtualMachine.Compile failed: %v", spew.Sdump(i))
		}
	}

}

func TestCompile(t *testing.T) {
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Compile(`a?{.abc}`)
	if err != nil {
		t.Fatal(err)
	}
	frame.Show()
}

func TestCompileFromDb(t *testing.T) {
	rulename := uuid.NewString() + ".sf"

	code := `
	a1 = {}
	a1.b = 1
	a2 = 3
	`
	syntaxflowRule := `
a*?{.b} as $a
	`

	_, err := sfdb.ImportRuleWithoutValid(rulename, syntaxflowRule, false)
	require.NoError(t, err)
	defer sfdb.DeleteRuleByRuleName(rulename)

	// check rule
	rule, err := sfdb.GetRule(rulename)
	require.NoError(t, err)
	require.NotEqual(t, rule.OpCodes, "")

	// check frame
	vm := sfvm.NewSyntaxFlowVirtualMachine()
	frame, err := vm.Load(rule)
	require.NoError(t, err)
	require.Greater(t, len(frame.Codes), 0)

	prog, err := ssaapi.Parse(code)
	require.NoError(t, err)

	t.Run("test frame feed", func(t *testing.T) {
		res, err := frame.Feed(prog)
		require.NoError(t, err)
		vs, ok := res.SymbolTable.Get("a")
		log.Infof(vs.String())
		require.True(t, ok)
		require.Contains(t, vs.String(), "make")
		require.NotContains(t, vs.String(), "3")
	})

	t.Run("test SyntaxFlowRule", func(t *testing.T) {
		res, err := prog.SyntaxFlowRule(rule)
		require.NoError(t, err)
		vs := res.GetValues("a")
		log.Infof(vs.String())
		require.Contains(t, vs.String(), "make")
		require.NotContains(t, vs.String(), "3")
	})

	t.Run("test SyntaxFlowRuleName", func(t *testing.T) {
		res, err := prog.SyntaxFlowRuleName(rulename)
		require.NoError(t, err)
		vs := res.GetValues("a")
		log.Infof(vs.String())
		require.Contains(t, vs.String(), "make")
		require.NotContains(t, vs.String(), "3")
	})

}
