package syntaxflow

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSF_Config_Filter(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1 
		ab = b 
		ac = c()
		`,
			"a* -{until:`* ?{opcode:const} `}-> * as $result",
			map[string][]string{
				"result": {"1"},
			})
	})

	t.Run("test data exchange between old and new VMs", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		a = 11
		a = f(a,1)
		b = f(a,2)
		b= 22
		`,
			"b #{hook:`* ?{opcode:const} as $num`}-> as $result",
			map[string][]string{
				"num":    {"1", "11", "2", "22"},
				"result": {"1", "11", "2", "22", "Undefined-f"},
			})
	})
}
