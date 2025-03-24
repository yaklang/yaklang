package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestProgramSyntaxFlow_Match(t *testing.T) {
	check := func(t *testing.T, sf, expect string) {
		code := `
		a = Runtime.getRuntime()
		a.exec("bash attack")
		`
		ssatest.CheckSyntaxFlow(t, code, sf, map[string][]string{
			"target": {expect},
		})
	}

	t.Run("Test Match", func(t *testing.T) {
		check(t, `Runtime as $target`, `Undefined-Runtime`)
	})

	t.Run("Match MemberCallMember", func(t *testing.T) {
		check(t, `Runtime.getRuntime as $target`, `Undefined-Runtime.getRuntime(valid)`)
	})

	t.Run("Match MemberCallMember with Call", func(t *testing.T) {
		check(t, `Runtime.getRuntime() as $target`, `Undefined-Runtime.getRuntime(valid)()`)
	})

	t.Run("Match MemberCallMember with Call nest", func(t *testing.T) {
		check(t, `Runtime.getRuntime().exec as $target`, `Undefined-a.exec(valid)`)
	})

	t.Run("only Match member call ", func(t *testing.T) {
		check(t, `.getRuntime as $target`, `Undefined-Runtime.getRuntime(valid)`)
	})
}

func TestGetVariable(t *testing.T) {
	code := `
	a1 = 1 
	a2 = 2 
	b1 = 3
	c1 = 4
	`

	t.Run("test from variable by name", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`a1 as $target`,
			map[string][]string{
				"target": {"1"},
			})
	})

	t.Run("by glob", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`a* as $target`,
			map[string][]string{
				"target": {"1", "2"},
			})
	})

	t.Run("by glob 2 ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`*1 as $target`,
			map[string][]string{
				"target": {"1", "3", "4"},
			})
	})

	t.Run("by regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`/(a1|b1)/ as $target`,
			map[string][]string{
				"target": {"1", "3"},
			})
	})

}

func TestGetMemberAndVariable(t *testing.T) {
	code := `
	obj = {
		"a1": 1, 
		"a2": 2, 
		"b1": 3,
	}
	a1 = 4 
	a2 = 5
	b1 = 6
	`

	t.Run("test from variable by name", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`a* as $target`,
			map[string][]string{
				"target": {"1", "2", "4", "5"},
			})
	})

	t.Run("test from variable name with default", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`a1 as $target`,
			map[string][]string{
				"target": {"1", "4"},
			})
	})

	t.Run("test from variable name with strict", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithSFOption(t, code,
			`a1 as $target`,
			map[string][]string{
				"target": {"4"},
			},
			ssaapi.QueryWithStrictMatch(),
		)
	})

	t.Run("test from variable key with default", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`.a1 as $target`,
			map[string][]string{
				"target": {"1"},
			})
	})

	t.Run("test from variable key with strict", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithSFOption(t, code,
			`.a1 as $target`,
			map[string][]string{
				"target": {"1"},
			},
			ssaapi.QueryWithStrictMatch(),
		)
	})

}

func TestGetMember(t *testing.T) {
	code := `
	obj = {
		"a1": 1, 
		"a2": 2, 
		"b1": 3,
	}
	`

	t.Run("test from variable by name", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`obj.a1 as $target`,
			map[string][]string{
				"target": {"1"},
			})
	})

	t.Run("test name", func(t *testing.T) {
		want := map[string][]string{
			"target": {"1"},
		}
		for _, sf := range []string{
			`*.a1 as $target`,
			`.a1 as $target`,
		} {
			ssatest.CheckSyntaxFlow(t, code, sf, want)
		}
	})

	t.Run("from variable by *", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`obj.* as $target`,
			map[string][]string{
				"target": {"1", "2", "3"},
			})
	})

	t.Run("from variable by regexp", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`obj.a* as $target`,
			map[string][]string{
				"target": {"1", "2"},
			})
	})

	t.Run("from variable by regexp 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`obj.*1 as $target`,
			map[string][]string{
				"target": {"1", "3"},
			})
	})
}

func TestMember_Multiple_member(t *testing.T) {
	code := `
	obj = {}
	obj.a = 1
	f1(obj.a)
	obj.a = 2 
	f2(obj.a)

	obj2  = {}
	obj2.a = 3 
	f
	`

	ssatest.CheckSyntaxFlow(t, code,
		`obj.a -> * as $target`,
		map[string][]string{
			"target": {
				"Undefined-f1(1)",
				"Undefined-f2(2)",
			},
		},
	)
}
