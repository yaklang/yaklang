package plugin_type_analyzer

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type PluginTypeAnalyzer interface {
	GetTypeSSAOpt() []ssaapi.Option
	// TODO: GetTypeInfo(*ssaapi.Program) // for grpc: pluginInfo
	CheckRule(*ssaapi.Program)
}

var pluginTypeAnalyzers = defaultAnalyzers()

func defaultAnalyzers() map[string]PluginTypeAnalyzer {
	return map[string]PluginTypeAnalyzer{
		"yak": &yakAnalyzer{},
	}
}

func GetPluginSSAOpt(pluginTyp string) []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	// all special plugin is yak.
	ret = append(ret, pluginTypeAnalyzers["yak"].GetTypeSSAOpt()...)
	if pluginTyp != "yak" {
		// if special plugin
		if rule, ok := pluginTypeAnalyzers[pluginTyp]; ok {
			ret = append(ret, rule.GetTypeSSAOpt()...)
		}
	}
	return ret
}

func CheckPluginType(pluginTyp string, prog *ssaapi.Program) {
	// all special plugin is yak.
	pluginTypeAnalyzers["yak"].CheckRule(prog)
	if pluginTyp != "yak" {
		// if special plugin
		if rule, ok := pluginTypeAnalyzers[pluginTyp]; ok {
			rule.CheckRule(prog)
		}
	}
}
