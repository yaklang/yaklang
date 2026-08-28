//go:build !ssa2llvm_aot

package main

import (
	builtin "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin"
	yaklib "github.com/yaklang/yaklang/common/yak/yaklib"
)

// registerRuntimeGlobals keeps the original full registration for non-AOT
// builds (tests and legacy archive builds), so the normal yaklang runtime
// behavior is unchanged.
func registerRuntimeGlobals() {
	runtimeRegisterYaklibGlobals(yaklib.GlobalExport)
	runtimeRegisterYaklibGlobals(builtin.YaklangBaseLib)
	runtimeRegisterYaklibGlobals(map[string]any{
		"len": runtimeYakBuiltinLen,
		"cap": runtimeYakBuiltinCap,
	})
}
