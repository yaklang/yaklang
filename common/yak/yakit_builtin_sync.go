package yak

import (
	"fmt"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func init() {
	// 强制同步内置资源到数据库（SyntaxFlow 规则、Core 插件、AI Forge），挂到 yakit 包供脚本调用
	yaklib.YakitExports["ForceSyncSyntaxFlowRule"] = forceSyncSyntaxFlowRule
	yaklib.YakitExports["ForceSyncCorePlugin"] = forceSyncCorePlugin
	yaklib.YakitExports["ForceSyncBuildInForge"] = forceSyncBuildInForge
}

// ForceSyncSyntaxFlowRule 强制把内置的 SyntaxFlow 规则同步到数据库（导出名为 yakit.ForceSyncSyntaxFlowRule）
// 适用于初始化或升级后刷新内置规则；可传入回调以接收同步进度
//
// 参数:
//   - notify: 进度回调 func(progress float64, message string)，可为 nil
//
// 返回值:
//   - 错误信息（同步失败时返回）
//
// Example:
// ```
// // 同步内置 SyntaxFlow 规则到数据库（会写库，示意性示例）
// yakit.ForceSyncSyntaxFlowRule(func(p, msg) { println(msg) })
// ```
func forceSyncSyntaxFlowRule(notify func(float64, string)) error {
	return sfbuildin.ForceSyncEmbedRule(notify)
}

// ForceSyncCorePlugin 强制把内置 Core 插件同步到数据库（导出名为 yakit.ForceSyncCorePlugin）
// 适用于初始化或升级后刷新内置插件；可传入回调以接收同步进度
//
// 参数:
//   - notify: 进度回调 func(progress float64, message string)，可为 nil
//
// 返回值:
//   - 错误信息（同步失败时返回）
//
// Example:
// ```
// // 同步内置 Core 插件到数据库（会写库，示意性示例）
// yakit.ForceSyncCorePlugin(func(p, msg) { println(msg) })
// ```
func forceSyncCorePlugin(notify func(float64, string)) error {
	if notify != nil {
		notify(0, "正在同步 Core 插件...")
	}
	err := coreplugin.ForceSyncCorePlugin()
	if notify != nil {
		notify(1, "Core 插件同步完成")
	}
	if err != nil {
		return fmt.Errorf("同步内置 Core 插件到数据库失败: %w", err)
	}
	return nil
}

// ForceSyncBuildInForge 强制把内置 AI Forge 同步到数据库（导出名为 yakit.ForceSyncBuildInForge）
// 适用于初始化或升级后刷新内置 AI Forge；可传入回调以接收同步进度
//
// 参数:
//   - notify: 进度回调 func(progress float64, message string)，可为 nil
//
// 返回值:
//   - 错误信息（同步失败时返回）
//
// Example:
// ```
// // 同步内置 AI Forge 到数据库（会写库，示意性示例）
// yakit.ForceSyncBuildInForge(func(p, msg) { println(msg) })
// ```
func forceSyncBuildInForge(notify func(float64, string)) error {
	if notify != nil {
		notify(0, "正在同步内置 AI Forge...")
	}
	err := aiforge.ForceSyncBuildInForge()
	if notify != nil {
		notify(1, "内置 AI Forge 同步完成")
	}
	if err != nil {
		return fmt.Errorf("同步内置 AI Forge 到数据库失败: %w", err)
	}
	return nil
}
