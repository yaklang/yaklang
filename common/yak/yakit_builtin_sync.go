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
	yaklib.YakitExports["ForceSyncSyntaxFlowRule"] = func(notify func(float64, string)) error {
		return sfbuildin.ForceSyncEmbedRule(notify)
	}
	yaklib.YakitExports["ForceSyncCorePlugin"] = func(notify func(float64, string)) error {
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
	yaklib.YakitExports["ForceSyncBuildInForge"] = func(notify func(float64, string)) error {
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
}
