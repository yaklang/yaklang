package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
)

func TestSSARuleMustPassCliDisable(t *testing.T) {
	t.Run("cli disable in mitm", func(t *testing.T) {
		check(t, `
		domains = cli.String("domains")
		cli.check()
			`,
			[]string{rules.ErrorDisableCLi(), rules.ErrorDisableCLi()},
			"mitm",
		)
	})

	t.Run("cli disable in yak", func(t *testing.T) {
		check(t, `
		domains = cli.String("domains")
		cli.check()
			`,
			[]string{},
			"yak",
		)
	})
}

func TestSSARuleMustPassMitmDisable(t *testing.T) {

	t.Run("test pack in mitm main with multiple same error", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		risk.cve("abc")
		risk.cve("abc")
		println(r)
		risk.Save(r)
			`,
			[]string{
				rules.MITMNotSupport("risk.CreateRisk"),
				rules.MITMNotSupport("risk.cve"),
				rules.MITMNotSupport("risk.cve"),
				rules.MITMNotSupport("risk.cve"),
				rules.MITMNotSupport("risk.Save"),
			},
			"mitm",
		)
	})

	t.Run("test pack in mitm main", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		println(r)
		risk.Save(r)
			`,
			[]string{
				rules.MITMNotSupport("risk.CreateRisk"),
				rules.MITMNotSupport("risk.cve"),
				rules.MITMNotSupport("risk.Save"),
			},
			"mitm",
		)
	})

	t.Run("test in mitm func", func(t *testing.T) {
		check(t, `
		() => {
			r = risk.CreateRisk(
				"abc",
				risk.cve("abc")
			)
			println(r)
			risk.Save(r)
		}
			`,
			[]string{},
			"mitm",
		)
	})

	t.Run("test in other plugin type", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		println(r)
		risk.Save(r)
			`,
			[]string{},
			"yak",
		)
	})

	t.Run("test Fuzz in MITM", func(t *testing.T) {
		check(t, `
		fuzz.HTTPRequest("")~

		fuzz.HTTPRequest("")~.Exec()
		fuzz.HTTPRequest("")~.FuzzGetParamsRaw().Exec()

		fuzz.HTTPRequest("")~.ExecFirst()
		fuzz.HTTPRequest("")~.FuzzGetParamsRaw().ExecFirst()
			`,
			[]string{
				rules.MITMNotSupport("fuzz.Exec or fuzz.ExecFirst"),
				rules.MITMNotSupport("fuzz.Exec or fuzz.ExecFirst"),
				rules.MITMNotSupport("fuzz.Exec or fuzz.ExecFirst"),
				rules.MITMNotSupport("fuzz.Exec or fuzz.ExecFirst"),
			},
			"mitm",
		)
	})
}
