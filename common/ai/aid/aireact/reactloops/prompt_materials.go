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

	// P1-C2: SessionEvidence (SESSION_ARTIFACTS) 与 UserHistory (PREV_USER_INPUT)
	// 是会话内"渐增列表", 历史进 dynamic 段会被 nonce 打散, 现已上移到
	// timeline-open 段 — 让最末几条变化时只刷新 timeline-open 段, 历史前缀仍可通
	// 过 frozen 边界跨调用命中。
	//
	// 关键词: PromptPrefixMaterials, SessionEvidence/UserHistory 上移 timeline-open,
	//        SESSION_ARTIFACTS frozen, PREV_USER_INPUT frozen, P1-C2
	SessionEvidence string
	UserHistory     string

	// RecentToolsCache 是 CACHE_TOOL_CALL 块 (directly_call_tool routing hint +
	// 最近工具 schema/footer) 的渲染输出, 用稳定 nonce 渲染, 字节级跨 turn 稳定.
	// 物理位置已从 dynamic 段 (REFLECTION) 迁到 semi-dynamic 段, 与 Skills + Schema
	// 一起被新增的 AI_CACHE_SEMI 边界包裹, 进入 prefix cache.
	//
	// 关键词: PromptPrefixMaterials, RecentToolsCache, semi-dynamic 段迁移
	RecentToolsCache string

	// FrozenUserContext 来自 LoopPromptAssemblyInput.FrozenUserContext, 承载
	// PE-TASK 的 PARENT_TASK + CURRENT_TASK + INSTRUCTION 三联块等 plan 周期内
	// 字节稳定的"用户级上下文"。
	//
	// 物理位置: 包装为 <|PLAN_CONTEXT_<plan-scoped-nonce>|>...<|PLAN_CONTEXT_END_
	// <plan-scoped-nonce>|> 后注入 timeline-open 段最末尾 (UserHistory 之后)。
	// timeline-open 段不被 AI_CACHE_FROZEN / AI_CACHE_SEMI 任何缓存边界包裹,
	// 是 prompt 的"易变尾段", 同 plan 周期内 PlanContext 内容随子任务切换
	// (CURRENT_TASK 内容变化) / EVIDENCE 演化 (root user input 嵌入更新) 抖动
	// 不再污染上游缓存。
	//
	// 历史:
	//   - v1: 老路径放 dynamic 段, 每 turn 用 turn nonce 渲染, 不可缓存;
	//   - v2: 迁到 frozen-block 段, 但本字段仅 PE-TASK 非空, root task / 普通
	//     ReAct 时为空, 渲染态抖动破坏 AI_CACHE_FROZEN 命中;
	//   - v3: 迁到 semi-dynamic 段, 但子任务切换 + EVIDENCE 嵌入 root user input
	//     仍让其内容抖动, 破坏 AI_CACHE_SEMI 命中;
	//   - v4 (当前): 迁到 timeline-open 段末尾, 主动让其落在所有 cache 边界外,
	//     不再追求 PlanContext 自身缓存, 而是保护更上游的 SYSTEM / FROZEN /
	//     SEMI 三段缓存稳定。
	//
	// 老路径 (普通 ReAct loop / focus mode 等) 此字段为空, timeline-open 段
	// PlanContext 子块自然不渲染, 段位置稳定。
	//
	// 关键词: PromptPrefixMaterials, FrozenUserContext, PLAN_CONTEXT 段,
	//        timeline-open 末尾注入, 缓存边界外, 上游缓存保护
	FrozenUserContext string
}

// HighStaticData 返回空 map: high-static 段已重构为完全无变量的纯静态系统提示词,
// 模板不再读取任何字段. 让该段跨 caller / 跨 turn 字节恒定, 最大化命中
// AI_CACHE_SYSTEM 边界. 历史的 AllowToolCall / AllowPlanAndExec /
// HasLoadCapability 三个能力开关已移除, 由 high-static 模板无条件介绍全部能力,
// 实际可用性以 SCHEMA enum 为准. TaskInstruction (PERSISTENT 块) 已迁到
// SemiDynamicData, 与 OutputExample 一起作为 caller-specific 的稳定 prefix.
//
// 关键词: HighStaticData 去变量化, AI_CACHE_SYSTEM 字节稳定, TaskInstruction 迁移
func (m *PromptPrefixMaterials) HighStaticData() map[string]any {
	return map[string]any{}
}

// SemiDynamicData 仅供 semi_dynamic_section.txt 模板消费, 渲染顺序为:
//   - SkillsContext: 已加载的 skills 上下文 (字节稳定)
//   - RecentToolsCache: CACHE_TOOL_CALL 块 (directly_call_tool routing hint +
//     最近工具 schema/footer), 用稳定 nonce 渲染, 字节稳定
//   - TaskInstruction: caller 注入的 PERSISTENT 指令, caller-specific, 从
//     high-static 段下沉到此处, 跨同一 caller 的 turn 字节稳定
//   - Schema: 当前 react loop 的 action schema (字节稳定 across turn)
//   - OutputExample: 当前 react loop 的输出示例, caller-specific, 不应污染
//     high-static 段; 紧跟 Schema 之后作为半动态 prefix cache 候选
//
// (Tool/Forge/Timeline frozen 已迁出到 FrozenBlock; CACHE_TOOL_CALL 已迁入此段;
// TaskInstruction 已从 high-static 迁入此段)
//
// PlanContext 已从此段移出 (曾短暂位于此段, 但因 PE-TASK 子任务切换和
// EVIDENCE 嵌入 root user input 引发抖动, 进一步迁到 timeline-open 段末尾,
// 落在所有 cache 边界之外, 见 TimelineOpenData)。
//
// 关键词: SemiDynamicData, Skills Context + RecentToolsCache + TaskInstruction +
//
//	Schema + OutputExample, PlanContext 已迁出至 timeline-open
func (m *PromptPrefixMaterials) SemiDynamicData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"SkillsContext":    m.SkillsContext,
		"Schema":           m.Schema,
		"OutputExample":    m.OutputExample,
		"TaskInstruction":  m.TaskInstruction,
		"RecentToolsCache": m.RecentToolsCache,
	}
}

// FrozenBlockData 供 frozen_block_section.txt 模板消费, 当前包含:
//   - ToolInventory + ForgeInventory: 系统级工具/forge 清单, 字节稳定
//   - TimelineFrozen: 时间轴 reducer + 非末 interval, 字节稳定
//
// 整个段被 AI_CACHE_FROZEN 边界包裹, hijacker 切到 user1 段进入 prefix cache.
//
// PlanContext 历史上曾在此段渲染, 但因仅 PE-TASK 子任务有内容, root task /
// 普通 ReAct loop 时为空, 这种"有时存在有时不存在"的渲染态会让 frozen-block
// 段在不同 task 类型下字节内容剧烈抖动, 破坏 AI_CACHE_FROZEN prefix cache 命中.
// 现已迁到 SemiDynamicData (semi-dynamic 段, AI_CACHE_SEMI 边界包裹), frozen-block
// 段只保留这三块跨 task 类型字节稳定的内容。
//
// 关键词: FrozenBlockData, Tool/Forge/Timeline-frozen,
//        AI_CACHE_FROZEN 块, PlanContext 已迁出
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
// Current Time + Workspace + (P1-C2: SessionEvidence / UserHistory 上移) +
// PlanContext 末尾注入。midterm 内容 (若有) 已并入 TimelineOpen。
//
// PlanContext (PE-TASK PLAN 产物 PARENT_TASK + CURRENT_TASK + INSTRUCTION +
// 父链 FACTS/DOCUMENT) 物理位置在本段最末尾 (UserHistory 之后)。本段不被
// AI_CACHE_FROZEN / AI_CACHE_SEMI 任何缓存边界包裹, 是 prompt 的"易变尾段",
// 让 PlanContext 的子任务切换抖动不会污染上游 system / frozen / semi 三段
// 缓存命中。
//
// 关键词: TimelineOpenData, Timeline 末桶, Current Time, Workspace,
//
//	SessionEvidence, UserHistory, P1-C2, PlanContext 末尾注入,
//	缓存边界外
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
		"SessionEvidence":  m.SessionEvidence,
		"UserHistory":      m.UserHistory,
		"PlanContext":      m.FrozenUserContext,
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
// 在新路径下 SemiDynamic = semi-dynamic 模板完整渲染串 (Skills + RecentToolsCache +
// TaskInstruction + Schema + OutputExample), Timeline = 旧合并 timeline 渲染
// (frozen + open 一起)。
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
