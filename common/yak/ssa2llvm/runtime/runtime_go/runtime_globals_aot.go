//go:build ssa2llvm_aot

package main

// registerRuntimeGlobals registers the minimal global set for the AOT runtime.
// print/println/printf/append are handled by the runtime dispatch table, so the
// globals map only needs the builtins the compiler emits directly. Keeping
// common/yak/yaklib and the yaklang builtin package out of the AOT build stops
// the whole yaklang frontend stack from being pulled into every binary.
func registerRuntimeGlobals() {
	runtimeRegisterYaklibGlobals(map[string]any{
		"len": runtimeYakBuiltinLen,
		"cap": runtimeYakBuiltinCap,
	})
}
