package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// LoopPromptBaseMaterials contains pre-rendered prompt ingredients supplied by
// the runtime so the loop can assemble prompt sections without reverse-parsing
// a monolithic background template.
//
// Timeline / TimelineFrozen / TimelineOpen 三个字段共存:
//   - Timeline: 老路径 (verification / tool-params 等) 仍消费的合并字符串, 与
//     timeline.Dump() 等价
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
	AIForgeList        string
	Timeline           string
	TimelineFrozen     string
	TimelineOpen       string
}

type LoopPromptAssemblyInput = aicommon.LoopPromptAssemblyInput

type LoopPromptAssemblyResult = aicommon.LoopPromptAssemblyResult

type PromptPrefixMaterials struct {
	Nonce             string
	AllowToolCall     bool
	AllowPlanAndExec  bool
	HasLoadCapability bool
	TaskInstruction   string
	OutputExample     string

	ToolInventory  bool
	ToolsCount     int
	TopToolsCount  int
	TopTools       []*aitool.Tool
	HasMoreTools   bool
	ForgeInventory bool
	AIForgeList    string
	SkillsContext  string
	Schema         string

	// Timeline 是兼容字段 (合并 frozen + open), 主路径不再消费, 保留给老 caller。
	Timeline string
	// TimelineFrozen / TimelineOpen 是按稳定性分层的两半, 主路径分别塞进
	// FrozenBlock 与 TimelineOpen 段。
	// 关键词: PromptPrefixMaterials, Timeline 拆分, frozen/open 分层
	TimelineFrozen   string
	TimelineOpen     string
	CurrentTime      string
	Workspace        bool
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
}

func (m *PromptPrefixMaterials) HighStaticData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"AllowToolCall":     m.AllowToolCall,
		"AllowPlanAndExec":  m.AllowPlanAndExec,
		"HasLoadCapability": m.HasLoadCapability,
		"TaskInstruction":   m.TaskInstruction,
		"OutputExample":     m.OutputExample,
	}
}

// SemiDynamicData 仅供 semi_dynamic_section.txt 模板消费, 现裁剪为
// Skills Context + Schema 两项 (Tool/Forge/Timeline frozen 已迁出到 FrozenBlock)。
//
// 关键词: SemiDynamicData, Skills Context + Schema, 重排后裁剪
func (m *PromptPrefixMaterials) SemiDynamicData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"SkillsContext": m.SkillsContext,
		"Schema":        m.Schema,
	}
}

// FrozenBlockData 供 frozen_block_section.txt 模板消费, 包含 Tool Inventory +
// Forge Inventory + Timeline-frozen 三块字节稳定内容。
//
// 关键词: FrozenBlockData, Tool/Forge/Timeline-frozen, AI_CACHE_FROZEN 块
func (m *PromptPrefixMaterials) FrozenBlockData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"ToolInventory":  m.ToolInventory,
		"ToolsCount":     m.ToolsCount,
		"TopToolsCount":  m.TopToolsCount,
		"TopTools":       m.TopTools,
		"HasMoreTools":   m.HasMoreTools,
		"ForgeInventory": m.ForgeInventory,
		"AIForgeList":    m.AIForgeList,
		"TimelineFrozen": m.TimelineFrozen,
	}
}

// TimelineOpenData 供 timeline_open_section.txt 模板消费, 包含 Timeline 末桶 +
// Current Time + Workspace。midterm 内容 (若有) 已并入 TimelineOpen。
//
// 关键词: TimelineOpenData, Timeline 末桶, Current Time, Workspace
func (m *PromptPrefixMaterials) TimelineOpenData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"TimelineOpen":     m.TimelineOpen,
		"CurrentTime":      m.CurrentTime,
		"Workspace":        m.Workspace,
		"OSArch":           m.OSArch,
		"WorkingDir":       m.WorkingDir,
		"WorkingDirGlance": m.WorkingDirGlance,
	}
}

// TimelineData 是老接口 (仍被部分 caller 调用), 等价于 TimelineOpenData 但回填
// Timeline 字段供老模板使用。新路径请优先用 TimelineOpenData / FrozenBlockData。
//
// 关键词: TimelineData, 兼容字段
func (m *PromptPrefixMaterials) TimelineData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"Timeline":         m.Timeline,
		"CurrentTime":      m.CurrentTime,
		"Workspace":        m.Workspace,
		"OSArch":           m.OSArch,
		"WorkingDir":       m.WorkingDir,
		"WorkingDirGlance": m.WorkingDirGlance,
	}
}

// PromptPrefixAssemblyResult 是 AssemblePromptPrefix 的输出。新路径下 Prompt
// 字段按 SYSTEM | FROZEN | SEMI | OPEN 顺序拼接; FrozenBlock / SemiDynamic /
// TimelineOpen 字段分别保留各段渲染串以便观测/测试断言。
//
// HighStatic / SemiDynamic / Timeline 是兼容字段 (用于老路径与单元测试),
// 在新路径下 SemiDynamic = SemiDynamic 残留段 (Skills + Schema), Timeline = 旧
// 合并 timeline 渲染 (frozen + open 一起)。
//
// 关键词: PromptPrefixAssemblyResult, 4 段拆分, 按稳定性分层
type PromptPrefixAssemblyResult struct {
	Prompt       string
	HighStatic   string
	FrozenBlock  string
	SemiDynamic  string
	TimelineOpen string
	Timeline     string
	Sections     []*PromptSectionObservation
}
