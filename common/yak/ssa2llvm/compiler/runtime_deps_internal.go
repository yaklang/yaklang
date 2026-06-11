package compiler

import (
	"sort"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
)

func runtimeYaklibDepsFromCompiler(comp *Compiler) []embed.YaklibDependency {
	if comp == nil {
		return nil
	}
	raw := comp.YaklibDependencies()
	if len(raw) == 0 {
		return nil
	}
	modules := make([]string, 0, len(raw))
	for module := range raw {
		modules = append(modules, module)
	}
	sort.Strings(modules)

	out := make([]embed.YaklibDependency, 0, len(modules))
	for _, module := range modules {
		methods := append([]string{}, raw[module]...)
		sort.Strings(methods)
		out = append(out, embed.YaklibDependency{
			Module:  module,
			Methods: methods,
		})
	}
	return out
}

func runtimeDepsFromCompiler(comp *Compiler) embed.PrunedRuntimeDependencies {
	if comp == nil {
		return embed.PrunedRuntimeDependencies{}
	}
	return embed.PrunedRuntimeDependencies{
		Yaklib:          runtimeYaklibDepsFromCompiler(comp),
		RuntimeDispatch: runtimeDispatchDepsFromCompiler(comp),
	}
}

func runtimeDispatchDepsFromCompiler(comp *Compiler) []abi.FuncID {
	if comp == nil {
		return nil
	}
	return comp.RuntimeDispatchDependencies()
}
