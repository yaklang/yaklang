package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"

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

func Test_Condition_FilterExpr(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (a1, a2) => {
			a1.b = 1
		}
		`,
			`
			a* as $target1
			$target1?{.b} as $target2
			a*?{.b} as $target3
			`,
			map[string][]string{
				"target1": {"Parameter-a1", "Parameter-a2"},
				"target2": {"Parameter-a1"},
				"target3": {"Parameter-a1"},
			})
	})

	t.Run("logical", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (a1, a2, a3) => {
			a1.b = 1
			a2.c = 2
		}
		`,
			`
			a* as $target1
			$target1?{(.b) || (.c)} as $target2
			a*?{(.b) || (.c)} as $target3
			`,
			map[string][]string{
				"target1": {"Parameter-a1", "Parameter-a2", "Parameter-a3"},
				"target2": {"Parameter-a1", "Parameter-a2"},
				"target3": {"Parameter-a1", "Parameter-a2"},
			})
	})
}
func TestConditionFilter(t *testing.T) {
	code := `
		f = (a1, a2, a3) => {
			a1 = "abc"
			b2 = "anc123"
			b3 = "anc"
			b4 = "anc1anc"
			a3 = 12
		}
`
	t.Run("test regexp condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
a* as $target
$target?{have: /^[0-9]+$/} as $output
`, map[string][]string{
			"output": {`12`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test global condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc1*} as $output
`, map[string][]string{
			"output": {`"anc123"`, `"anc1anc"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test exact condition", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
a* as $target
$target?{have: abc} as $output
`, map[string][]string{
			"output": {`"abc"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test global and exact", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc,*123} as $output
`, map[string][]string{
			"output": []string{`"anc123"`},
		})
	})
	t.Run("test exact and regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc,/[0-9]+$/} as $output
`, map[string][]string{
			"output": {`"anc123"`},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test global and regex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
b* as $target
$target?{have: anc*,/[0-9]+$/} as $output
`, map[string][]string{
			"output": {`"anc123"`},
		})
	})
}
