package plugin_type_analyzer

import (
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/ssaapi"

	// for init function
	_ "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/rules"
	_ "github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/ssa_option"
)

// plugin type : "yak" "mitm" "port-scan" "codec"

func GetPluginSSAOpt(plugin string) []ssaapi.Option {
	ret := plugin_type.GetPluginSSAOpt(plugin_type.PluginTypeYak)
	pluginType := plugin_type.ToPluginType(plugin)
	if pluginType != plugin_type.PluginTypeYak {
		ret = append(ret, plugin_type.GetPluginSSAOpt(pluginType)...)
	}
	return ret
}

func CheckPluginType(plugin string, prog *ssaapi.Program) {
	plugin_type.CheckPluginType(plugin_type.PluginTypeYak, prog)
	pluginType := plugin_type.ToPluginType(plugin)
	if pluginType != plugin_type.PluginTypeYak {
		plugin_type.CheckPluginType(pluginType, prog)
	}
}
