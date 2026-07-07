//go:build !irify_exclude

package yak

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// initIrifyLibs 初始化 irify_exclude 构建下会剥离的库（SSA、SyntaxFlow、NASL）。
func initIrifyLibs() {
	yaklang.Import("nasl", antlr4nasl.Exports)

	// ssa
	ssaExports := []map[string]any{
		ssaapi.Exports,
		ssaproject.Exports,
		ssaconfig.Exports,
	}
	yaklang.Import("ssa", lo.Assign(ssaExports...))
	// SyntaxFlow
	sfExports := []map[string]any{
		syntaxflow.Exports,
		syntaxflow_scan.Exports,
	}
	yaklang.Import("syntaxflow", lo.Assign(sfExports...))
	yaklang.Import("sfreport", sfreport.Exports)
}
