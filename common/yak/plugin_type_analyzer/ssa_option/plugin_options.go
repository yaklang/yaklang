package ssa_option

import (
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func init() {
	plugin_type.RegisterSSAOptCollector(plugin_type.PluginTypeMitm, MitmGetTypeSSAOpt)
	plugin_type.RegisterSSAOptCollector(plugin_type.PluginTypeCodec, CodecSSAOpt)
	plugin_type.RegisterSSAOptCollector(plugin_type.PluginTypePortScan, ProtScanSSAOpt)
}

func MitmGetTypeSSAOpt() []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)

	// mitm
	valueTable := make(map[string]interface{})
	valueTable["MITM_PLUGIN"] = ""
	valueTable["MITM_PARAMS"] = make(map[string]string)
	ret = append(ret, ssaapi.WithExternValue(valueTable))
	ret = append(ret, ssaapi.WithExternInfo("plugin-type:mitm"))

	return ret
}

func CodecSSAOpt() []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	return ret
}

func ProtScanSSAOpt() []ssaapi.Option {
	ret := make([]ssaapi.Option, 0)
	return ret
}
