package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSimple(t *testing.T) {
	t.Run("TestSimple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1 // constant
		ab = b // undefined
		`,
			`
		a* as $target1
		$target1?{opcode: const} as $target2
		`,
			map[string][]string{
				"target1": {"1", "Undefined-ab"},
				"target2": {"1"},
			},
		)
	})

	t.Run("Test multiple opcode", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = 1 // constant
		ab = b // undefined
		f = (i) => {
			ac = i
		}
		`,
			`
		a* as $target1
		$target1?{opcode: const, param} as $target2
		`,
			map[string][]string{
				"target1": {"1", "Undefined-ab", "Parameter-i"},
				"target2": {"1", "Parameter-i"},
			},
		)
	})

}
