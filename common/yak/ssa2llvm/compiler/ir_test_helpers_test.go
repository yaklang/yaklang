package compiler

import "github.com/yaklang/yaklang/common/yak/ssaapi"

func compileToIRFromCodeWithExternBindings(code, language string, bindings map[string]ExternBinding) (*ssaapi.Program, *Compiler, string, error) {
	prog, comp, ir, err := compileInput("", code, language, bindings, nil, nil)
	if comp != nil {
		comp.Dispose()
	}
	return prog, nil, ir, err
}
