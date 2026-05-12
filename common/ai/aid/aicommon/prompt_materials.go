package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

// PromptMaterials 是 prefix 渲染的统一语义材料模型。
//
// 设计目标:
//   - 作为 aireact 与 aid planAndExec 的共同 prefix 输入
//   - 明确承载 high-static / frozen / semi / timeline-open 各层语义字段
//   - PromptPrefixBuilder 只接收这一种语义材料，再由各段模板按需消费
//
// 关键词: PromptMaterials, shared prefix materials, aireact + aid 共用
type PromptMaterials struct {
	Nonce             string
	AllowToolCall     bool
	AllowPlanAndExec  bool
	HasLoadCapability bool

	TaskInstruction string
	Schema          string
	OutputExample   string

	// SemiDynamic 提示材料:
	//   - aireact: SkillsContext + RecentToolsCache
	//   - aid: PlanHelp / OriginalUserInput / StableInstruction 等 prompt-specific
	//     但仍属可缓存半动态前缀的内容
	SkillsContext     string
	RecentToolsCache  string
	PlanHelp          string
	OriginalUserInput string
	StableInstruction string

	ToolInventory  bool
	ToolsCount     int
	TopToolsCount  int
	TopTools       []*aitool.Tool
	HasMoreTools   bool
	ForgeInventory bool
	AIForgeList    string

	// Timeline 是兼容字段 (合并 frozen + open), 老 caller 仍可消费。
	Timeline string

	TimelineFrozen    string
	TimelineOpen      string
	CurrentTime       string
	Workspace         bool
	OSArch            string
	WorkingDir        string
	WorkingDirGlance  string
	SessionEvidence   string
	UserHistory       string
	FrozenUserContext string
}

// HighStaticData 返回空 map: high-static 段是完全无变量的系统级共享 static。
func (m *PromptMaterials) HighStaticData() map[string]any {
	return map[string]any{}
}

// SemiDynamicData 供 caller-specific semi-dynamic 模板消费。
func (m *PromptMaterials) SemiDynamicData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"SkillsContext":     m.SkillsContext,
		"RecentToolsCache":  m.RecentToolsCache,
		"PlanHelp":          m.PlanHelp,
		"OriginalUserInput": m.OriginalUserInput,
		"StableInstruction": m.StableInstruction,
	}
}

// SemiDynamic1Data 兼容 aireact P1.1 命名。
func (m *PromptMaterials) SemiDynamic1Data() map[string]any {
	return m.SemiDynamicData()
}

// SemiDynamic2Data 供 TaskInstruction -> Schema -> OutputExample 半动态段消费。
func (m *PromptMaterials) SemiDynamic2Data() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"TaskInstruction": m.TaskInstruction,
		"Schema":          m.Schema,
		"OutputExample":   m.OutputExample,
	}
}

// FrozenBlockData 供 frozen-block 模板消费。
func (m *PromptMaterials) FrozenBlockData() map[string]any {
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

// TimelineOpenData 供 timeline-open 模板消费。
func (m *PromptMaterials) TimelineOpenData() map[string]any {
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
		"SessionEvidence":  m.SessionEvidence,
		"UserHistory":      m.UserHistory,
		"PlanContext":      m.FrozenUserContext,
	}
}

// TimelineData 是老路径兼容字段。
func (m *PromptMaterials) TimelineData() map[string]any {
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
