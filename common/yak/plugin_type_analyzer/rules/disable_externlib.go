package rules

import (
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
		v.NewError(tag, "CLI does not support this type")
	})
}

func ErrorDisableTypeCLi() string {
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
	check := func(v *ssaapi.Value) {
		if v.InMainFunction() {
			v.NewError(tag, "MITM does not support these packs")
		}
	}

	for _, v := range DisablePack {
		prog.Ref(v).GetDefs().GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(check)
	}

	DisableFunction := []string{"fuzz.Exec", "fuzz.ExecFirst"}
	for _, v := range DisableFunction {
		prog.Ref(v).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return v.IsCall() && v.IsReachable() != -1
		}).ForEach(check)
	}
}

func ErrorDisableMitmPacks() string {
	return "MITM does not support these packs"
}

func ErrorDisableMitmFunctions() string {
	return "MITM does not support these functions"
}
