package syntaxflow

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestError(t *testing.T) {

	prog, err := ssaapi.Parse(`println("aaa")`)
	require.NoError(t, err)

	syntaxflowRules := []string{
		`$a() as $call_a`,
		`alert $a`,
		`check $a`,
	}

	for _, rule := range syntaxflowRules {
		t.Run(fmt.Sprintf("rule: %s", rule), func(t *testing.T) {
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			_ = res
		})
	}
}
