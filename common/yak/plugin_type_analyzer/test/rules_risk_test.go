package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
)

func TestSSARuleMustPassRiskOption(t *testing.T) {
	t.Run("risk with nothing", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc"
		)
			`, []string{
			rules.ErrorRiskCheck(),
		})
	})
	t.Run("risk with cve", func(t *testing.T) {
		check(t, `
	risk.NewRisk(
		"abc", 
		risk.cve("abc")
	)
		`, []string{})
	})
	t.Run("risk with description and solution", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc"),
			risk.description("abc")
		)
		`, []string{})
	})
	t.Run("risk with description", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.description("abc")
		)
			`, []string{
			rules.ErrorRiskCheck(),
		})
	})
	t.Run("risk with solution", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc")
		)
			`, []string{
			rules.ErrorRiskCheck(),
		})
	})
	t.Run("risk with all", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc"),
			risk.description("abc"),
			risk.cve("abc")
		)
			`, []string{})
	})

}

func TestSSARuleMustPassRiskCreate(t *testing.T) {
	t.Run("risk create not use", func(t *testing.T) {
		check(t, `
		risk.CreateRisk(
			"abc", 
			risk.cve("abc")
		)
			`, []string{
			rules.ErrorRiskCreateNotSave(),
		})
	})

	t.Run("risk create used but not saved", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc", 
			risk.cve("abc")
		)
		println(r)
			`, []string{
			rules.ErrorRiskCreateNotSave(),
		})
	})

	t.Run("risk create saved", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc", 
			risk.cve("abc")
		)
		risk.Save(r)
			`, []string{})
	})

	t.Run("risk create saved and used", func(t *testing.T) {
		check(t, `
		r = risk.CreateRisk(
			"abc",
			risk.cve("abc")
		)
		println(r)
		risk.Save(r)
			`, []string{})
	})

}
