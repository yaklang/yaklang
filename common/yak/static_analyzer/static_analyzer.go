package static_analyzer

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yaklang"

	// for init function
	_ "github.com/yaklang/yaklang/common/yak/static_analyzer/rules"
	_ "github.com/yaklang/yaklang/common/yak/static_analyzer/ssa_option"
)

// plugin type : "yak" "mitm" "port-scan" "codec"

func StaticAnalyzeYaklang(code, codeTyp string) []*result.StaticAnalyzeResult {
	var results []*result.StaticAnalyzeResult

	// compiler
	newEngine := yaklang.New()
	newEngine.SetStrictMode(false)
	_, err := newEngine.Compile(code)
	if err != nil {
		switch ret := err.(type) {
		case yakast.YakMergeError:
			for _, e := range ret {
				results = append(results, &result.StaticAnalyzeResult{
					Message:         fmt.Sprintf("基础语法错误（Syntax Error）：%v", e.Message),
					Severity:        result.Error,
					StartLineNumber: int64(e.StartPos.LineNumber),
					StartColumn:     int64(e.StartPos.ColumnNumber + 1),
					EndLineNumber:   int64(e.EndPos.LineNumber),
					EndColumn:       int64(e.EndPos.ColumnNumber + 2),
					From:            "compiler",
				})
			}
		default:
			log.Error("静态分析失败：Yaklang 返回错误不标准")
		}
	}

	prog, err := ssaapi.Parse(code, GetPluginSSAOpt(codeTyp)...)
	if err != nil {
		log.Error("SSA 解析失败：", err)
		return results
	}
	results = append(results, checkPluginType(codeTyp, prog).Get()...)

	errs := prog.GetErrors()
	for _, err := range errs {
		severity := result.Hint
		switch err.Kind {
		case ssa.Warn:
			severity = result.Warn
		case ssa.Error:
			severity = result.Error
		}
		results = append(results, &result.StaticAnalyzeResult{
			Message:         err.Message,
			Severity:        severity,
			StartLineNumber: int64(err.Pos.Start.Line),
			StartColumn:     int64(err.Pos.Start.Column + 1),
			EndLineNumber:   int64(err.Pos.End.Line),
			EndColumn:       int64(err.Pos.End.Column + 1),
			From:            "SSA:" + string(err.Tag),
		})
	}
	return results
}

func GetPluginSSAOpt(plugin string) []ssaapi.Option {
	ret := plugin_type.GetPluginSSAOpt(plugin_type.PluginTypeYak)
	pluginType := plugin_type.ToPluginType(plugin)
	if pluginType != plugin_type.PluginTypeYak {
		ret = append(ret, plugin_type.GetPluginSSAOpt(pluginType)...)
	}
	return ret
}

func checkPluginType(plugin string, prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults()
	ret.Merge(plugin_type.CheckPluginType(plugin_type.PluginTypeYak, prog))
	pluginType := plugin_type.ToPluginType(plugin)
	if pluginType != plugin_type.PluginTypeYak {
		ret.Merge(plugin_type.CheckPluginType(pluginType, prog))
	}
	return ret
}
