package antlr4nasl

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type NaslScriptConfig struct {
	plugins              []string
	family               string
	proxies              []string
	riskHandle           func(risk any)
	conditions           map[string]any
	preference           map[string]any
	autoLoadDependencies bool
}

func NewNaslScriptConfig() *NaslScriptConfig {
	return &NaslScriptConfig{
		autoLoadDependencies: true,
		preference:           make(map[string]any),
		conditions:           make(map[string]any),
	}
}

type NaslScriptConfigOptFunc func(c *NaslScriptConfig)

func WithPreference(p interface{}) NaslScriptConfigOptFunc {
	preference := utils.InterfaceToMapInterface(p)
	return func(c *NaslScriptConfig) {
		c.preference = preference
	}
}
func WithConditions(script ...any) NaslScriptConfigOptFunc {
	queryCondition := map[string]any{}
	if len(script) > 0 {
		for k, v := range utils.InterfaceToMapInterface(script[0]) {
			if utils.StringArrayContains([]string{"origin_file_name", "cve", "script_name", "category", "family"}, k) {
				queryCondition[k] = v
			} else {
				log.Warnf("not allow query field %s", k)
			}
		}
	}
	return func(c *NaslScriptConfig) {
		c.conditions = queryCondition
	}
}

func WithProxy(proxies ...string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.proxies = proxies
	}
}
func WithRiskHandle(f func(any)) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.riskHandle = f
	}
}
func WithFamily(family string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.family = family
	}
}

func WithPlugins(plugins ...string) NaslScriptConfigOptFunc {
	return func(c *NaslScriptConfig) {
		c.plugins = plugins
	}
}
