package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
)

func TestRulesDefineFunction(t *testing.T) {

	t.Run("test no implement define function in codec", func(t *testing.T) {
		check(t,
			`a = 1
			`,
			[]string{
				rules.NoImplementFunction("handle"),
			},
			"codec",
		)
	})

	t.Run("test duplicate define function in codec", func(t *testing.T) {
		check(t,
			`
			handle = (a) => a
			handle = (a) => a
			`,
			[]string{
				rules.DuplicateFunction("handle"),
				rules.DuplicateFunction("handle"),
			},
			"codec",
		)
	})

	t.Run("test no implement define function in mitm", func(t *testing.T) {
		funcs := []string{
			"hijackSaveHTTPFlow",
			"hijackHTTPResponse",
			"hijackHTTPResponseEx",
			"hijackHTTPRequest",
			"mirrorNewWebsitePathParams",
			"mirrorNewWebsitePath",
			"mirrorNewWebsite",
			"mirrorFilteredHTTPFlow",
			"mirrorHTTPFlow",
		}

		check(t,
			` a= 1`,
			[]string{
				rules.LeastImplementOneFunctions(funcs),
			},
			"mitm")
	})

	t.Run("test implement define function in mitm", func(t *testing.T) {
		check(t,
			` 
			hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
				a = 1
			}
			`,
			[]string{},
			"mitm")
	})

	t.Run("test duplicate define function in mitm", func(t *testing.T) {
		check(t,
			` 
			hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
				a = 1
			}
			hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
				b = 1
			}
			`,
			[]string{
				rules.DuplicateFunction("hijackSaveHTTPFlow"),
				rules.DuplicateFunction("hijackSaveHTTPFlow"),
			},
			"mitm")
	})

}
