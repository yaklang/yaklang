package rules

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

func init() {
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeYak, Deprecated)
}

func Deprecated(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	res := result.NewStaticAnalyzeResults("deprecated check")

	handler := func(funName, msg string) {
		prog.Ref(funName).GetUsers().Filter(func(v *ssaapi.Value) bool {
			return !v.IsExternLib()
		}).ForEach(func(v *ssaapi.Value) {
			res.NewDeprecated(msg, v)
		})
	}

	funcs := doc.DefaultDocumentHelper.DeprecatedFunctions
	for _, fun := range funcs {
		handler(fun.Name, fun.Msg)
	}
	return res
}
