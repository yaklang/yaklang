package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"

	"github.com/yaklang/yaklang/common/yak/static_analyzer/rules"
)

func TestSSARuleMustPassRiskOption(t *testing.T) {
	t.Run("risk with nothing", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc"
		)
			`, []string{
			rules.ErrorRiskCheck("risk.NewRisk"),
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
			rules.ErrorRiskCheck("risk.NewRisk"),
		})
	})
	t.Run("risk with solution", func(t *testing.T) {
		check(t, `
		risk.NewRisk(
			"abc", 
			risk.solution("abc")
		)
			`, []string{
			rules.ErrorRiskCheck("risk.NewRisk"),
		})
	})
	t.Run("risk with desc and solution", func(t *testing.T) {
		code := `
		risk.NewRisk(
			"abc", 
			risk.solution("abc"),
			risk.description("abc")
		)
			`
		check(t, code, []string{})
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

func TestSSARuleMustPassNewRiskPosition(t *testing.T) {
	t.Run("test invalid risk save location (mitm) ", func(t *testing.T) {
		check(t, `
		 server,token = risk.NewDNSLogDomain()~
		 handle = func(result) {
				log.Info("result: ", result)
		}
			`, []string{rules.ErrorInvalidRiskNewLocation()}, string(plugin_type.PluginTypeMitm))
	})

	t.Run("test invalid risk save location (port-scan)", func(t *testing.T) {
		check(t, `
		 	risk.NewRisk("http://example.com", risk.title("SQL注入漏洞"), risk.type("sqli"), risk.severity("high"), risk.description(""), risk.solution(""))
			handleCheck = func(target,port){
				 addr = str.HostPort(target, port)
			}
			handle = func(result) {
				log.Info("result: ", result)
			}
			`, []string{rules.ErrorInvalidRiskNewLocation()}, string(plugin_type.PluginTypePortScan))
	})

	t.Run("test invalid risk save location (codec)", func(t *testing.T) {
		check(t, `
		 server,token = risk.NewHTTPLog()~
		 handle = func(result) {
				log.Info("result: ", result)
			}
			`, []string{rules.ErrorInvalidRiskNewLocation()}, string(plugin_type.PluginTypeCodec))
	})

	t.Run("test correct risk save location ", func(t *testing.T) {
		check(t, `
		handleCheck = func(target,port){
			addr = str.HostPort(target, port)
			isTls = str.IsTLSServer(addr)
			server,token = risk.NewDNSLogDomain()~
		}
		handle = func(result) {
				log.Info("result: ", result)
			}
			`, []string{}, string(plugin_type.PluginTypePortScan))
	})
}
