package rules

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func init() {
	// disable
	// plugin type : "yak" "mitm" "port-scan" "codec"
	plugin_type_analyzer.RegisterCheckRuler("mitm", DisableCli)
	plugin_type_analyzer.RegisterCheckRuler("port-scan", DisableCli)
	plugin_type_analyzer.RegisterCheckRuler("codec", DisableCli)

	plugin_type_analyzer.RegisterCheckRuler("mitm", DisableMitmExternLib)
}

func DisableCli(prog *ssaapi.Program) {
	tag := "SSA-cli-disableType"
	prog.Ref("cli").GetDefs().GetUsers().Filter(func(v *ssaapi.Value) bool {
		return v.IsCall() && v.IsReachable() != -1
	}).ForEach(func(v *ssaapi.Value) {
		v.NewError(tag, ErrorDisableCLi())
	})
}

func ErrorDisableCLi() string {
	return "CLI does not support this type"
}

func DisableMitmExternLib(prog *ssaapi.Program) {
	tag := "SSA-cli-disableMitmExternLib"
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
					v.NewError(tag, MITMNotSupport(Func.GetName()))
				}
			})
		})
	}

	DisableFunction := []string{"fuzz.Exec", "fuzz.ExecFirst"}
	for _, funName := range DisableFunction {
		prog.Ref(funName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(func(v *ssaapi.Value) {
			if v.InMainFunction() {
				v.NewError(tag, MITMNotSupport(funName))
			}
		})
	}
}

func MITMNotSupport(pkg string) string {
	return fmt.Sprintf("MITM does not support %s in main function", pkg)
}
