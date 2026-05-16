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

	TimelineFrozen   string
	TimelineOpen     string
	CurrentTime      string
	Workspace        bool
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
	// SessionArtifactsListing 是会话工件目录的结构化清单 (path / size /
	// mtime, 按 task 分组 + mtime desc), 由 RenderSessionArtifactsListing
	// 生成. 之前作为 ContextProviderManager 的 "session_artifacts" 注册项
	// 落到 Pure Dynamic / AutoContext 段, 现已下沉到 Workspace 块, 与 OS /
	// working dir / glance 一起渲染, 段位仍属 timeline-open.
	// 关键词: SessionArtifactsListing, Workspace 内嵌, Pure Dynamic 反污染
	SessionArtifactsListing string
	SessionEvidence         string
	UserHistory             string
	FrozenUserContext       string
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

// TimelineOpenData 供 timeline-open 模板消费, 模板字段渲染顺序 (P1-C3):
//
//	Timeline (Open Tail) -> SessionEvidence -> Workspace -> UserHistory ->
//	Current Time -> PlanContext (末尾)
//
// 段内排序原则:
//  1. Timeline (Open Tail) 在最前: 时间线最末桶是模型理解"刚发生了什么"的
//     首要信息源, 顶到段首让 LLM 第一时间看到。midterm 内容 (若有) 已并入
//     TimelineOpen。
//  2. SessionEvidence 紧跟 Timeline: SESSION_ARTIFACTS 是 Config 级持久化
//     观测 (跨 turn 累积的工件证据), 与 Timeline 末桶共同构成"会话级实证"
//     连续语料块, 物理上贴近 Timeline 让两者形成连续语义。
//  3. Workspace 居中: OS/Arch + working dir + glance 是相对静态的环境标识,
//     既不属于"刚发生", 也不属于"用户视角", 居中过渡。
//  4. UserHistory 在 Workspace 之后: PREV_USER_INPUT 是用户历史输入轨迹,
//     与下方 Current Time 一起构成"时序前缀"。
//  5. Current Time 紧跟 UserHistory: 当前时间是最末稳定的时序锚点, 形成
//     "历史输入 -> 现在"时间递进, 同时与下方 PlanContext 形成"时间 ->
//     任务"语义衔接。
//  6. PlanContext (PE-TASK PLAN 产物 PARENT_TASK + CURRENT_TASK + INSTRUCTION
//     + 父链 FACTS/DOCUMENT) 在段最末尾。本段不被 AI_CACHE_FROZEN /
//     AI_CACHE_SEMI 任何缓存边界包裹, 是 prompt 的"易变尾段", 让 PlanContext
//     的子任务切换抖动不会污染上游 system / frozen / semi 三段缓存命中。
//
// 注: Go map literal 的 key 顺序不影响模板渲染 (template 按 key 取值),
// 这里 key 顺序与上面文档中的渲染顺序保持一致只是为了源码可读性, 真正的
// 渲染顺序由 prompts/prefix/timeline_open_section.txt 决定。
//
// 关键词: TimelineOpenData, Timeline 末桶, SessionEvidence, Workspace,
//
//	UserHistory, Current Time, PlanContext 末尾注入, P1-C3 段内顺序,
//	缓存边界外
func (m *PromptMaterials) TimelineOpenData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"TimelineOpen":            m.TimelineOpen,
		"SessionEvidence":         m.SessionEvidence,
		"Workspace":               m.Workspace,
		"OSArch":                  m.OSArch,
		"WorkingDir":              m.WorkingDir,
		"WorkingDirGlance":        m.WorkingDirGlance,
		"SessionArtifactsListing": m.SessionArtifactsListing,
		"UserHistory":             m.UserHistory,
		"CurrentTime":             m.CurrentTime,
		"PlanContext":             m.FrozenUserContext,
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
