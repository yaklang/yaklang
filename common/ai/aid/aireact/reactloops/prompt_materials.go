package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// LoopPromptBaseMaterials contains pre-rendered prompt ingredients supplied by
// the runtime so the loop can assemble prompt sections without reverse-parsing
// a monolithic background template.
//
// Timeline 已经按稳定性分层为 frozen / open 两段:
//   - TimelineFrozen: 仅 reducer + 非末 interval 的"冻结前缀"渲染, 不带边界 tag
//   - TimelineOpen: 仅最末 interval 桶 + midterm prefix 等"易变尾段", 不带边界 tag
//
// 关键词: LoopPromptBaseMaterials, Timeline 拆分, frozen/open 分层
type LoopPromptBaseMaterials struct {
	Nonce              string
	Language           string
	TaskType           string
	ForgeName          string
	AllowPlanAndExec   bool
	AllowToolCall      bool
	HasLoadCapability  bool
	ShowForgeInventory bool
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	AutoContext        string
	UserHistory        string
	ToolsCount         int
	TopToolsCount      int
	TopTools           []*aitool.Tool
	HasMoreTools       bool
	// MoreToolsCount = ToolsCount - TopToolsCount, 即 token 预算裁剪后没能
	// 进入 Tool Inventory 列表的剩余工具数量, 由 aicommon.SelectToolsByTokenBudget
	// 计算结果设置, 让模板能给出具体数字而不是模糊的 "...".
	// 关键词: MoreToolsCount, Tool Inventory 剩余工具数, search_capabilities 具象提示
	MoreToolsCount int
	AIForgeList    string
	aicommon.PromptFrozenOpenMaterials
}

type LoopPromptAssemblyInput = aicommon.LoopPromptAssemblyInput

type LoopPromptAssemblyResult = aicommon.LoopPromptAssemblyResult

// PromptPrefixMaterials 已收敛到 aicommon.PromptMaterials。
// 保留 alias 只是兼容 reactloops 现有调用面，不再维护第二套顶层语义模型。
//
// 段内顺序与字段语义已由 aicommon/prompt_materials.go 集中维护，相关注释
// (P1-C3 段内排序原则等) 也迁移到 aicommon 包中。
//
// 关键词: PromptPrefixMaterials alias, aicommon.PromptMaterials,
//
//	prompt 语义材料统一收敛
type PromptPrefixMaterials = aicommon.PromptMaterials

// PromptPrefixAssemblyResult 是 AssemblePromptPrefix 的输出。新路径下 Prompt
// 字段按 SYSTEM | FROZEN | SEMI-1 | SEMI-2 | OPEN 顺序拼接;
// FrozenBlock / SemiDynamic1 / SemiDynamic2 / TimelineOpen 字段分别保留各段
// 渲染串以便观测/测试断言。
//
// HighStatic / SemiDynamic1 / SemiDynamic2 是兼容字段 (用于老路径与单元测试):
//   - SemiDynamic1 = semi_dynamic_section_1.txt 完整渲染串
//     (SkillsContext + RecentToolsCache),
//     被 wrapAICacheSemi 包一层 AI_CACHE_SEMI 边界, 物理上对应 hijacker 5 段切分
//     中的 user2 (不打 cc).
//   - SemiDynamic2 = semi_dynamic_section_2.txt 完整渲染串
//     (TaskInstruction + Schema + OutputExample),
//     被 wrapAICacheSemi2 包一层 AI_CACHE_SEMI2 边界, 物理上对应 hijacker 5 段切分
//     中的 user3 (主动打 ephemeral cc), 与 SemiDynamic1 合并算 prefix cache.
//
// 关键词: PromptPrefixAssemblyResult, 5 段拆分, 按稳定性分层, P1.1 三 cache 边界
type PromptPrefixAssemblyResult struct {
	Prompt       string
	HighStatic   string
	FrozenBlock  string
	SemiDynamic1 string
	SemiDynamic2 string
	TimelineOpen string
	Sections     []*PromptSectionObservation
}
