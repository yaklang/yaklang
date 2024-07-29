package java

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestGraphFrom_XXE(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		assert.Equal(t, prog.GetLanguage(), "java")
		results := prog.SyntaxFlowChain(`
desc("Description": 'checking setFeature/setXIncludeAware/setExpandEntityReferences in DocumentBuilderFactory.newInstance()')

DocumentBuilderFactory.newInstance()?{!((.setFeature) || (.setXIncludeAware) || (.setExpandEntityReferences))} as $entry;
$entry.*Builder().parse(* #-> as $source);

check $source then "XXE Attack" else "XXE Safe";
`).DotGraph()
		if !utils.MatchAllOfSubString(
			results,
			"fontcolor", "color",
			"step[",
			"penwidth=\"3.0\"",
			": call",
			"search parse",
			"all-actual-args",
		) {
			fmt.Println(results)
			t.Fatal("failed to match all of the substring, bad dot graph")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}
