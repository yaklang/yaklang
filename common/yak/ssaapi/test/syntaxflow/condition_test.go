package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSimple(t *testing.T) {
	t.Run("Test opcode", func(t *testing.T) {
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

	t.Run("string condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		`,
			`
		a* as $target1
		$target1?{have: abc} as $target2
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`},
				"target2": {`"abcccc"`},
			},
		)
	})

	t.Run("negative condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		ac = b // undefined
		`,
			`
		a* as $target1
		$target1?{not have: abc} as $target2
		$target1?{! opcode: const} as $target3
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, "Undefined-ac"},
				"target2": {`"araaa"`, "Undefined-ac"},
				"target3": {"Undefined-ac"},
			},
		)
	})

	t.Run("logical condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = abcccc()
		ac = "abcccc"
		`,
			`
		a* as $target1
		$target1?{(have: abc) && (opcode: const)} as $target2
		$target1?{(! have: ara) && ((have: abc) || (opcode: const))} as $target3
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, "Undefined-abcccc", "Undefined-abcccc()"},
				"target2": {`"abcccc"`},
				"target3": {"Undefined-abcccc()", "Undefined-abcccc", `"abcccc"`},
			},
		)
	})

}

func Test_String_Contain(t *testing.T) {
	t.Run("test string contain have", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		ac = "ccc"
		`,
			`
		a* as $target1
		$target1?{have: abc, ccc} as $target2
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, `"ccc"`},
				"target2": {`"abcccc"`},
			},
		)
	})

	t.Run("test string contain any", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		aa = "araaa"
		ab = "abcccc"
		ac = "ccc"
		`,
			`
		a* as $target1
		$target1?{any: abc, ccc} as $target2
		`,
			map[string][]string{
				"target1": {`"araaa"`, `"abcccc"`, `"ccc"`},
				"target2": {`"abcccc"`, `"ccc"`},
			},
		)
	})
}
