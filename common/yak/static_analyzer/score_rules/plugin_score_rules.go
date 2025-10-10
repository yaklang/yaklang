package score_rules

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func init() {
	plugin_type.RegisterScoreCheckRuler(plugin_type.PluginTypeMitm, CheckDefineFunctionMitm)
	plugin_type.RegisterScoreCheckRuler(plugin_type.PluginTypeYak, ForbidExecLib)
}

func ForbidExecLib(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("forbid command exec library")
	prog.Ref("exec").ForEach(func(value *ssaapi.Value) {
		if value.IsExternLib() {
			newErr := ret.NewError(LibForbid("exec"), value)
			newErr.SetNegativeScore(100)
		}
	})
	return ret
}

func CheckDefineFunctionMitm(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("check define function in mitm")
	funcs := []string{
		"analyzeHTTPFlow",
		"onAnalyzeHTTPFlowFinish",
		"hijackSaveHTTPFlow",
		"hijackHTTPResponse",
		"hijackHTTPResponseEx",
		"hijackHTTPRequest",
		"mirrorNewWebsitePathParams",
		"mirrorNewWebsitePath",
		"mirrorNewWebsite",
		"mirrorFilteredHTTPFlow",
		"mirrorHTTPFlow",
	}

	find := false
	for _, name := range funcs {
		defineFuncs := prog.SyntaxFlow(fmt.Sprintf("%s?{opcode: function} as $fun", name)).GetValues("fun")
		if len(defineFuncs) == 0 {
			// not implement
			continue
		}
		// implement
		find = true

		if len(defineFuncs) != 1 {
			// duplicate
			defineFuncs.ForEach(func(v *ssaapi.Value) {
				// v.NewWarn(CheckDefineFunctionTag, DuplicateFunction(name))
				result := ret.NewWarn(DuplicateFunction(name), v)
				result.SetNegativeScore(100)
			})
		}
		fun := defineFuncs[0]
		hasCode := false
		if f, ok := ssa.ToFunction(fun.GetSSAInst()); ok {
			for _, block := range f.Blocks {
				b, ok := f.GetBasicBlockByID(block)
				if !ok || b == nil {
					continue
				}
				if len(b.Insts) > 0 {
					hasCode = true
					break
				}
			}
		}
		if !hasCode {
			result := ret.NewError(FunctionEmpty(name), fun)
			result.SetNegativeScore(100)
		}

	}

	if !find {
		result := ret.NewError(LeastImplementOneFunctions(funcs), nil)
		result.SetNegativeScore(100)
	}
	return ret
}

func DuplicateFunction(name string) string {
	return fmt.Sprintf("function [%s] duplicate implement", name)
}

func LeastImplementOneFunctions(name []string) string {
	return fmt.Sprintf("At least implement one function: %v", name)
}

func FunctionEmpty(name string) string {
	return fmt.Sprintf("Function [%s] is empty, should implement this function", name)
}

func LibForbid(name string) string {
	return fmt.Sprintf("Library [%s] is forbidden", name)
}
