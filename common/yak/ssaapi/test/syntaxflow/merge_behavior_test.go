package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSameVariableOutputShouldMerge(t *testing.T) {
	code := `
	a1 = {}
	a2 = {}
	`
	rule := `
a1 as $merged
a2 as $merged
	`

	ssatest.CheckResult(t, code, rule, func(res *ssaapi.SyntaxFlowResult) {
		require.Len(t, res.GetValues("merged"), 2, "same variable output should merge instead of overwrite")
	}, nil, []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.Yak)})
}

func TestMergedValuesCanBeFilteredByCondition(t *testing.T) {
	code := `
	a1 = {}
	a1.b = 1
	a2 = {}
	a3 = {}
	a3.b = 1
	`
	rule := `
a* as $merged
$merged?{.b} as $filtered
	`

	ssatest.CheckResult(t, code, rule, func(res *ssaapi.SyntaxFlowResult) {
		require.Len(t, res.GetValues("merged"), 3, "merged variable should keep all matched values")
		require.Len(t, res.GetValues("filtered"), 2, "condition filter should keep only values having .b")
	}, nil, []ssaconfig.Option{ssaapi.WithLanguage(ssaconfig.Yak)})
}
