package syntaxflow_test

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed syntaxflow_include_test.lib.sf
var sflib string

func TestSFLib(t *testing.T) {
	const ruleName = "fetch-abc-calling"
	_, err := sfdb.ImportRuleWithoutValid(ruleName, `
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

func TestFS_RuleUpdate(t *testing.T) {
	name := "yak-a.sf"
	content := `
	desc(lib: "a")
	a as $a
	alert $a
	`
	sfdb.ImportRuleWithoutValid(name, content, true)
	defer sfdb.DeleteRuleByRuleName(name)

	ssatest.CheckSyntaxFlow(t, `
	a = 1 
	b = 2`,
		`
	<include(a)> as $target
	`, map[string][]string{
			"target": {"1"},
		},
	)

	// update
	content = `
	desc(lib: "b")
	b as $a
	alert $a
	`
	sfdb.ImportRuleWithoutValid(name, content, true)

	ssatest.CheckSyntaxFlow(t, `
	a = 1 
	b = 2`,
		`
	<include(b)> as $target
	`, map[string][]string{
			"target": {"2"},
		},
	)
}
