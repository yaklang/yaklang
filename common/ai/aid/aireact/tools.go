package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// getPrioritizedTools returns a prioritized list of tools, with search tools first.
//
// extraPriority 是可变参数 (常见用例: yak focus mode 通过 __SCENARIO_TOOLS__
// dunder 把指定的 scenario 工具拉回 inventory 时, 同步在这里把这些名字
// 置顶, 让它们出现在最前). extraPriority 的优先级高于内置 priorityNames,
// 即 whitelist 命中的工具会被排在 search_capabilities 等之前.
//
// 关键词: getPrioritizedTools, extraPriority, scenario whitelist top,
//
//	focus mode pull back ordering
func (r *ReAct) getPrioritizedTools(tools []*aitool.Tool, maxCount int, extraPriority ...string) []*aitool.Tool {
	return aicommon.PrioritizeToolsForInventory(tools, maxCount, extraPriority...)
}

func NewTestReAct(opts ...aicommon.ConfigOption) (*ReAct, error) {
	basicOption := []aicommon.ConfigOption{
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
	}
	basicOption = append(basicOption, opts...)
	ins, err := NewReAct(
		basicOption...,
	)
	if err != nil {
		return nil, err
	}
	ins.memoryTriage.SetInvoker(ins)
	ins.config.SetConfig("test_yaklang_aikb_rag", true)
	return ins, nil
}
