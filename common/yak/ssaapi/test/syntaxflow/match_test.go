package syntaxflow

import (
	"testing"

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
