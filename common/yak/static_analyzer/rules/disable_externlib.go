package rules

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func init() {
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeMitm, DisableCli)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypePortScan, DisableCli)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeCodec, DisableCli)

	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeMitm, DisableMitmExternLib)
}

func DisableCli(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults()
	// tag := "SSA-cli-disableType"
	prog.Ref("cli").GetDefs().GetUsers().Filter(func(v *ssaapi.Value) bool {
		return v.IsCall() && v.IsReachable() != -1
	}).ForEach(func(v *ssaapi.Value) {
		ret.NewError(ErrorDisableCLi(), v)
	})
	return ret
}

func ErrorDisableCLi() string {
	return "This PluginType does not support CLI package"
}

func DisableMitmExternLib(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults()
	// tag := "SSA-cli-disableMitmExternLib"
	// 在MITM插件禁用 risk、poc、http、tcp、udp、fuzz.Exec、fuzz.ExecFirst
	DisablePack := []string{
		"risk",
		"poc",
		"http",
		"tcp",
		"udp"}

	for _, pkgName := range DisablePack {
		prog.Ref(pkgName).GetDefs().ForEach(func(Func *ssaapi.Value) {
			Func.GetUsers().Filter(func(v *ssaapi.Value) bool {
				return v.IsCall() && v.IsReachable() != -1
			}).ForEach(func(v *ssaapi.Value) {
				if v.InMainFunction() {
					ret.NewError(MITMNotSupport(Func.GetName()), v)
				}
			})
		})
	}

	DisableFunction := []string{
		// for "fuzz.Exec"
		// interface
		"github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequestIf.Exec",
		// implement
		"github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequest.Exec",
		"github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequestBatch.Exec",
		//  for "fuzz.ExecFirst"
		"github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequestIf.ExecFirst",
		// implement
		"github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequest.ExecFirst",
		"github.com/yaklang/yaklang/common/mutate.FuzzHTTPRequestBatch.ExecFirst",
	}
	for _, funName := range DisableFunction {
		prog.Ref(funName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			if v.InMainFunction() {
				ret.NewError(MITMNotSupport("fuzz.Exec or fuzz.ExecFirst"), v)
			}
		})
	}
	return ret
}

func MITMNotSupport(pkg string) string {
	return fmt.Sprintf("MITM does not support %s in main function", pkg)
}
