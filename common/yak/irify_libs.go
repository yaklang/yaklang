//go:build !irify_exclude

package yak

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaproject"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// initIrifyLibs 初始化 Irify 相关的库（SSA 和 SyntaxFlow）
// 仅在非 irify_exclude 模式下调用
func initIrifyLibs() {
	// ssa
	ssaExports := []map[string]any{
		ssaapi.Exports,
		ssaproject.Exports,
		ssaconfig.Exports,
	}
	yaklang.Import("ssa", lo.Assign(ssaExports...))
	yaklang.Import("syntaxflow", syntaxflow.Exports)
}
