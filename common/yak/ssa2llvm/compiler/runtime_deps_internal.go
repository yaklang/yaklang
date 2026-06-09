package compiler

import (
	"sort"

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
