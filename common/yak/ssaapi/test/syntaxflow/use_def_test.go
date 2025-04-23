package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSF_GetUser(t *testing.T) {
	check := func(t *testing.T, sf string, expect []string) {
		code := `
		a = Runtime.getRuntime()
		result := a.exec("bash attack")
		b = file.Write("abc", result)
		dump(b)
		`
		ssatest.CheckSyntaxFlowContain(t, code, sf, map[string][]string{
			"target": expect,
		})
	}

	t.Run("Test get user use memberCall", func(t *testing.T) {
		check(t,
			`Runtime.getRuntime().exec->* as $target`,
			[]string{`Undefined-a.exec(valid)("bash attack")`},
		)
	})

	t.Run("Test get user use variable", func(t *testing.T) {
		check(t,
			`a.exec->* as $target`,
			[]string{`Undefined-a.exec(valid)("bash attack")`},
		)
	})

	t.Run("Test get bottom user with memberCall", func(t *testing.T) {
		check(t,
			`Runtime.getRuntime().exec-->* as $target`,
			[]string{
				"Undefined-dump(Undefined-file.Write(valid)(\"abc\",Undefined-a.exec(valid)(\"bash attack\")))",
			},
		)
	})

	t.Run("Test get bottom user with variable", func(t *testing.T) {
		check(t,
			`a.exec-->* as $target`,
			[]string{
				"Undefined-dump(Undefined-file.Write(valid)(\"abc\",Undefined-a.exec(valid)(\"bash attack\")))",
			},
		)
	})

	t.Run("test get def memberCall", func(t *testing.T) {
		check(t,
			`Runtime.getRuntime().exec()#>* as $target`,
			[]string{
				`Undefined-a.exec(valid)`,
				`"bash attack"`,
			},
		)
	})

	t.Run("test get def by variable", func(t *testing.T) {
		check(t,
			`a.exec()#>* as $target`,
			[]string{
				`Undefined-a.exec(valid)`,
				`"bash attack"`,
			},
		)
	})

	t.Run("test get top def", func(t *testing.T) {
		check(t,
			`b#->* as $target`,
			[]string{
				"\"abc\"", "\"bash attack\"", "Undefined-Runtime", "Undefined-file"},
		)
	})
}
