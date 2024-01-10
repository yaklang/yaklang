package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
)

func TestSSARuleMustPassYakCliParamName(t *testing.T) {
	t.Run("cli same paramName", func(t *testing.T) {
		check(t, `
cli.String("a")
cli.String("a")
cli.check()
	`, []string{rules.ErrorStrSameParamName("a", 2)})
	})

	t.Run("cli invalid paramName", func(t *testing.T) {
		check(t, `
cli.String("!@#")
cli.String("a")
cli.check()
	`, []string{rules.ErrorStrInvalidParamName("!@#")})
	})
}

func TestSSARuleMustPassYakCliCheck(t *testing.T) {
	t.Run("cli with check", func(t *testing.T) {
		check(t, `
	cli.String("a")
	cli.check()
		`, []string{})
	})
	t.Run("cli not check", func(t *testing.T) {
		check(t, `
		cli.String("a")
			`, []string{
			rules.ErrorStrNotCallCliCheck(),
		})
	})
	t.Run("cli not check in last", func(t *testing.T) {
		check(t, `
		cli.String("a")
		cli.check()
		cli.String("b")
			`, []string{
			rules.ErrorStrNotCallCliCheck(),
		})
	})
	t.Run("not cli function", func(t *testing.T) {
		check(t, `
			println("aaaa")
			`, []string{})
	})
}
