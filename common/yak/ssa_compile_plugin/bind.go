// Package ssa_compile_plugin registers yak-plugin execution for ssa_compile.
//
// syntaxflow_scan imports ssa_compile (compile in-process via ssaapi) and must
// not import this package: yakscript → yak → syntaxflow_scan would cycle.
package ssa_compile_plugin

import (
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/yakscript"
)

func init() {
	ssa_compile.BindYakPluginExec(yakscript.ExecScriptWithParam)
}
