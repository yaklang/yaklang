package rules

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/contextmenu"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/plugin_type"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func init() {
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeCodec, CheckDefineFunctionCodec)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypePortScan, CheckDefineFunctionPortScan)
	plugin_type.RegisterCheckRuler(plugin_type.PluginTypeContextMenu, CheckDefineFunctionContextMenu)
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

func CheckDefineFunctionContextMenu(prog *ssaapi.Program) *result.StaticAnalyzeResults {
	ret := result.NewStaticAnalyzeResults("check context-menu hooks")
	implemented := 0
	for _, actionID := range contextmenu.KnownActions() {
		hookName, _ := contextmenu.HookForAction(actionID)
		handlers := prog.Ref(hookName).Filter(func(v *ssaapi.Value) bool { return v.IsFunction() })
		if len(handlers) == 0 {
			continue
		}

		implemented++
		if len(handlers) > 1 {
			handlers.ForEach(func(v *ssaapi.Value) {
				ret.NewError(DuplicateFunction(hookName), v)
			})
			continue
		}

		want := contextmenu.ExpectedParameterCount(actionID)
		got := len(handlers[0].GetParameters())
		if got != want {
			ret.NewError(InvalidFunctionParameterCount(hookName, want, got), handlers[0])
		}
	}

	if implemented == 0 {
		ret.NewError(AtLeastOneContextMenuHook(), nil)
	}
	return ret
}

func AtLeastOneContextMenuHook() string {
	return "at least one context-menu hook must be implemented"
}

func InvalidFunctionParameterCount(name string, want, got int) string {
	return fmt.Sprintf("function [%s] requires %d parameters, got %d", name, want, got)
}
