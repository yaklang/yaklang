package rules

import (
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func init() {
	plugin_type_analyzer.RegisterSSAOptCollector("mitm", MitmGetTypeSSAOpt)
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
