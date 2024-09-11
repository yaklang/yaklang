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

type StaticAnalyzeKind uint8

const (
	Analyze StaticAnalyzeKind = iota
	Score
)

// plugin type : "yak" "mitm" "port-scan" "codec"

func StaticAnalyzeYaklang(code, codeTyp string, kind StaticAnalyzeKind) []*result.StaticAnalyzeResult {
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
					StartColumn:     int64(e.StartPos.ColumnNumber),
					EndLineNumber:   int64(e.EndPos.LineNumber),
					EndColumn:       int64(e.EndPos.ColumnNumber + 1),
					From:            "compiler",
				})
			}
		default:
			log.Error("静态分析失败：Yaklang 返回错误不标准")
		}
	}

	prog, err := SSAParse(code, codeTyp)
	if err != nil {
		log.Error("SSA 解析失败：", err)
		return results
	}
	results = append(results, checkRules(codeTyp, prog, kind).Get()...)

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
			StartLineNumber: int64(err.Pos.GetStart().GetLine()),
			StartColumn:     int64(err.Pos.GetStart().GetColumn()),
			EndLineNumber:   int64(err.Pos.GetEnd().GetLine()),
			EndColumn:       int64(err.Pos.GetEnd().GetColumn()),
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

func checkRules(plugin string, prog *ssaapi.Program, kind StaticAnalyzeKind) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults()
	switch kind {
	case Score:
		ret.Merge(plugin_type.CheckScoreRules(plugin_type.PluginTypeYak, prog))
		pluginType := plugin_type.ToPluginType(plugin)
		if pluginType != plugin_type.PluginTypeYak {
			ret.Merge(plugin_type.CheckScoreRules(pluginType, prog))
		}
		fallthrough
	default:
		ret.Merge(plugin_type.CheckRules(plugin_type.PluginTypeYak, prog))
		pluginType := plugin_type.ToPluginType(plugin)
		if pluginType != plugin_type.PluginTypeYak {
			ret.Merge(plugin_type.CheckRules(pluginType, prog))
		}
	}

	return ret
}

func SSAParse(code, scriptType string, o ...ssaapi.Option) (*ssaapi.Program, error) {
	opt := GetPluginSSAOpt(scriptType)
	opt = append(opt, ssaapi.WithEnableCache())
	opt = append(opt, o...)
	return ssaapi.Parse(code, opt...)
}
