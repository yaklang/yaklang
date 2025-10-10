package ssa_option

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
)

func init() {
	plugin_type.RegisterSSAOptCollector(plugin_type.PluginTypeMitm, MitmGetTypeSSAOpt)
	plugin_type.RegisterSSAOptCollector(plugin_type.PluginTypeCodec, CodecSSAOpt)
	plugin_type.RegisterSSAOptCollector(plugin_type.PluginTypePortScan, ProtScanSSAOpt)
}

func MitmGetTypeSSAOpt() []ssaapi.Option {
	ret := make([]ssaapi.Option, 0, 3)

	// mitm
	valueTable := make(map[string]interface{})
	valueTable["MITM_PLUGIN"] = ""
	valueTable[consts.PLUGIN_CONTEXT_KEY_RUNTIME_ID] = ""
	valueTable["MITM_PARAMS"] = make(map[string]string)
	ret = append(ret, ssaapi.WithExternValue(valueTable))

	ret = append(ret, ssaapi.WithDefineFunc(map[string]any{
		"analyzeHTTPFlow": func(flow *schema.HTTPFlow, extract func(ruleName string, flow *schema.HTTPFlow, contents ...string)) {
		},
		"onAnalyzeHTTPFlowFinish":    func(totalCount int64, matchedCount int64) {},
		"hijackSaveHTTPFlow":         func(flow *schema.HTTPFlow, modify func(*schema.HTTPFlow), drop func()) {},
		"hijackHTTPResponse":         func(isHttps bool, url string, rsp []byte, forward func([]byte), drop func()) {},
		"hijackHTTPResponseEx":       func(isHttps bool, url string, req []byte, rsp []byte, forward func([]byte), drop func()) {},
		"hijackHTTPRequest":          func(isHttps bool, url string, req []byte, forward func([]byte), drop func()) {},
		"mirrorNewWebsitePathParams": func(isHttps bool, url string, req, rsp, body []byte) {},
		"mirrorNewWebsitePath":       func(isHttps bool, url string, req, rsp, body []byte) {},
		"mirrorNewWebsite":           func(isHttps bool, url string, req, rsp, body []byte) {},
		"mirrorFilteredHTTPFlow":     func(isHttps bool, url string, req, rsp, body []byte) {},
		"mirrorHTTPFlow":             func(isHttps bool, url string, req, rsp, body []byte) {},
	}))

	ret = append(ret, ssaapi.WithExternInfo("plugin-type:mitm"))
	return ret
}

func CodecSSAOpt() []ssaapi.Option {
	ret := make([]ssaapi.Option, 0, 2)
	ret = append(ret, ssaapi.WithDefineFunc(map[string]any{
		"handle": func(string) string { return "" },
	}))
	ret = append(ret, ssaapi.WithExternInfo("plugin-type:codec"))
	return ret
}

func ProtScanSSAOpt() []ssaapi.Option {
	ret := make([]ssaapi.Option, 0, 2)
	ret = append(ret, ssaapi.WithDefineFunc(map[string]any{
		"handle": func(*fp.MatchResult) {},
	}))
	ret = append(ret, ssaapi.WithExternInfo("plugin-type:portscan"))
	return ret
}
