package rules

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func init() {
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeCodec, CheckDefineFunctionCodec)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypePortScan, CheckDefineFunctionPortScan)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeMitm, CheckDefineFunctionMitm)
}

// var CheckDefineFunctionTag string = "CheckDefineFunction"

func checkDefineFunction(prog *ssaapi.Program, name string) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("check define function")
	handlers := prog.Ref(name).Filter(func(v *ssaapi.Value) bool { return v.IsFunction() })
	if len(handlers) == 0 {
		ret.NewError(NoImplementFunction(name), nil)
	} else if len(handlers) > 1 {
		handlers.ForEach(func(v *ssaapi.Value) {
			ret.NewError(DuplicateFunction(name), v)
		})
	}
	return ret
}

func NoImplementFunction(name string) string {
	return fmt.Sprintf("function [%s] not implement", name)
}

func DuplicateFunction(name string) string {
	return fmt.Sprintf("function [%s] duplicate implement", name)
}

func checkFreeValue(fun *ssaapi.Value) {
	if !fun.IsFunction() {
		return
	}

}

func CheckDefineFunctionCodec(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	return checkDefineFunction(prog, "handle")
}

func CheckDefineFunctionPortScan(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	return checkDefineFunction(prog, "handle")
}

func CheckDefineFunctionMitm(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("check define function in mitm ")
	funcs := []string{
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
				ret.NewWarn(DuplicateFunction(name), v)
			})
		}
		fun := defineFuncs[0]
		hasCode := false
		if f, ok := ssa.ToFunction(ssaapi.GetBareNode(fun)); ok {
			for _, block := range f.Blocks {
				b, ok := ssa.ToBasicBlock(block)
				if !ok {
					continue
				}
				if len(b.Insts) > 0 {
					hasCode = true
					break
				}
			}
		}
		if !hasCode {
			// ret.NewError(FunctionEmpty(name), fun)
			/* TODO: check function data has code,
			1.	this rule should be evaluate not static-analyze
				should in file: common/yakgrpc/grpc_smoking_evaluate_plugin.go
			2. 	this rule, should check other yak-plugin: (mitm, port-scan, codec)
			*/
		}

	}

	if !find {
		ret.NewError(LeastImplementOneFunctions(funcs), nil)
	}
	return ret
}

func LeastImplementOneFunctions(name []string) string {
	return fmt.Sprintf("At least implement one function: %v", name)
}

func FunctionEmpty(name string) string {
	return fmt.Sprintf("Function [%s] is empty, should implement this function", name)
}
