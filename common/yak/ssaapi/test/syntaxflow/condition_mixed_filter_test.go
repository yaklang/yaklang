package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_MixedFilterAndOpcode(t *testing.T) {
	code := `
f = (p) => {
	x = p.b
	y = p.c
	return x
}
obj = {
	b: 11,
	c: 22,
}
r = f(obj)
`

	t.Run("param_with_member_b", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `p?{opcode:param && .b} as $target`, map[string][]string{
			"target": {"Parameter-p"},
		})
	})

	t.Run("param_with_member_c", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `p?{opcode:param && .c} as $target`, map[string][]string{
			"target": {"Parameter-p"},
		})
	})

	t.Run("param_with_member_not_exist", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `p?{opcode:param && .not_exist} as $target`, map[string][]string{
			"target": {},
		})
	})
}

func TestCondition_FilterLogicalBoundary(t *testing.T) {
	code := `
a = {
	b: 1,
	c: 2,
}
x = a
`

	t.Run("field_and_field_match", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `x?{.b && .c} as $target`, map[string][]string{
			"target": {"a"},
		})
	})

	t.Run("field_and_field_not_match", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `x?{.b && .d} as $target`, map[string][]string{
			"target": {},
		})
	})

	t.Run("multi_source_should_not_expand_from_predecessor", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
x1 = {
	b: {
		c: 1,
	},
}
x2 = {
	b: {
		e: 2,
	},
}
`, `x*?{.b.c} as $target`, map[string][]string{
			"target": {"x1"},
		})
	})
}
