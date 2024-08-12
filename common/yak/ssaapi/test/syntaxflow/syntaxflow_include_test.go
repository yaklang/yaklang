package syntaxflow

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed syntaxflow_include_test.lib.sf
var sflib string

func TestSFLib(t *testing.T) {
	const ruleName = "fetch-abc-calling"
	err := sfdb.ImportRuleWithoutValid(ruleName, `
desc(lib: "abc");
abc() as $output;
alert $output
`, false)
	if err != nil {
		t.Fatal(err)
	}
	defer sfdb.DeleteRuleByRuleName(ruleName)

	ssatest.Check(t, `

abc = () => {
	return "abc"
}

e = d(abc())
dump(e)

`, func(prog *ssaapi.Program) error {
		results := prog.SyntaxFlowChain("<include(abc)> --> *").Show()
		if len(results) < 1 {
			t.Fatal("no result")
		}
		return nil
	})
}
