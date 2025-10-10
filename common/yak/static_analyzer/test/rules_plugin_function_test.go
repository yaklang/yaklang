package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/rules"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/score_rules"
)

func TestScoreRulesDefineFunction(t *testing.T) {
	t.Run("test implement define function in mitm, but empty ", func(t *testing.T) {
		checkScore(t,
			`
			mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
			}
			`,
			[]string{
				score_rules.FunctionEmpty("mirrorHTTPFlow"),
				"empty block",
			},
			0,
			"mitm")
	})

	t.Run("test no implement define function in mitm", func(t *testing.T) {
		funcs := []string{
			"analyzeHTTPFlow",
			"onAnalyzeHTTPFlowFinish",
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

		checkScore(t,
			` a= 1`,
			[]string{
				score_rules.LeastImplementOneFunctions(funcs),
			},
			0,
			"mitm")
	})

	t.Run("test implement define function in mitm", func(t *testing.T) {
		checkScore(t,
			` 
			hijackSaveHTTPFlow = func(flow /* *schema.HTTPFlow */, modify /* func(modified *schema.HTTPFlow) */, drop/* func() */) {
				a = 1
			}
			`,
			[]string{},
			100,
			"mitm")
	})

	t.Run("test duplicate define function in mitm", func(t *testing.T) {
		checkScore(t,
			` 
			hijackSaveHTTPFlow = func(flow /* *schema.HTTPFlow */, modify /* func(modified *schema.HTTPFlow) */, drop/* func() */) {
				a = 1
			}
			hijackSaveHTTPFlow = func(flow /* *schema.HTTPFlow */, modify /* func(modified *schema.HTTPFlow) */, drop/* func() */) {
				b = 1
			}
			`,
			[]string{
				rules.DuplicateFunction("hijackSaveHTTPFlow"),
				rules.DuplicateFunction("hijackSaveHTTPFlow"),
			},
			0,
			"mitm")
	})
}

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
	t.Run("test use and define function", func(t *testing.T) {
		check(t, `
		_test_ = () => {
			hijackSaveHTTPFlow()
		}
		hijackSaveHTTPFlow = func(flow /* *schema.HTTPFlow */, modify /* func(modified *schema.HTTPFlow) */, drop/* func() */) {
			b = 1
		}
		`, []string{
			ssa4analyze.FreeValueUndefine("hijackSaveHTTPFlow"),
		}, "mitm")

		check(t, `
		_test_ = () => {
			handle()
		}
		handle = (s) => {
			return s
		}
		`, []string{
			ssa4analyze.FreeValueUndefine("handle"),
		}, "codec")
	})
}

func TestRuleDefineFunctionWithFreeValue(t *testing.T) {
	t.Run("can't find FreeValue", func(t *testing.T) {
		check(t,
			`
	handle = result => {
		println(a)
		return result
	}
		`,
			[]string{
				ssa4analyze.FreeValueUndefine("a"),
				ssa.ValueUndefined("a"),
			},
			"codec",
		)
	})

	t.Run("can find FreeValue", func(t *testing.T) {
		check(
			t, `
filter = 1
handle = r => {
	println(filter)
	return ""
}
			`,
			[]string{}, "codec",
		)
	})

	t.Run("can find FreeValue with local variable", func(t *testing.T) {
		check(
			t, `
filter := 1
handle = r => {
	println(filter)
	return ""
}
			`,
			[]string{}, "codec",
		)
	})
}

func TestScoreRulesForbidLib(t *testing.T) {
	t.Run("test forbid exec lib", func(t *testing.T) {
		checkScore(t,
			` exec.Command("")`,
			[]string{
				score_rules.LibForbid("exec"),
			},
			0,
			"yak")
	})
}
