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
	FunctionCallMode  bool
	AllowToolCall     bool
	AllowPlanAndExec  bool
	HasLoadCapability bool

	TaskInstruction     string
	ExecutionPolicy     string
	Schema              string
	FunctionCallSchemas string
	OutputExample       string

	// SemiDynamic 提示材料:
	//   - aireact: SkillsContext
	//   - aid: PlanHelp / OriginalUserInput / StableInstruction 等 prompt-specific
	//     但仍属可缓存半动态前缀的内容
	SkillsContext        string
	PromotedSemiDynamic1 string
	PlanHelp             string
	OriginalUserInput    string
	StableInstruction    string

	// ForcedSkills 是「用户强制加载」SKILL 满内容, 进 frozen_block 顶部 (最高优先级).
	// 空时 frozen_block 顶部子块不渲染.
	ForcedSkills string
	// AutoLoadedSkills 是「AI 意图驱动加载」SKILL, 进 semi_dynamic_2 尾部.
	// 空时 semi_dynamic_2 尾部子块不渲染.
	AutoLoadedSkills string

	ToolInventory bool
	ToolsCount    int
	TopToolsCount int
	TopTools      []*aitool.Tool
	HasMoreTools  bool
	// MoreToolsCount = ToolsCount - TopToolsCount, 即未在 prompt Top 列表中
	// 渲染的剩余工具数量. 让模板能直接展开成具体数字 ("...still 73 more tools
	// available via search_capabilities"), 而不是只给一个无信息的省略号.
	// 关键词: MoreToolsCount, Tool Inventory 剩余工具数
	MoreToolsCount int
	ForgeInventory bool
	AIForgeList    string

	TimelineFrozen         string
	TimelineOpen           string
	PromotedTimelineOpen   string
	TimelineFrozenTimeUnix int64
	FrozenPartitions       []FrozenBlockPartition
	SessionArtifactsFrozen string
	SessionArtifactsOpen   string
	SessionEvidenceFrozen  string
	SessionEvidenceOpen    string
	CurrentTime            string
	Workspace              bool
	OSArch                 string
	WorkingDir             string
	WorkingDirGlance       string
	AIArtifactsDir         string

	// Deprecated: Session Artifacts no longer participate in prompt construction.
	SessionArtifactsListing string
	// Deprecated: SessionEvidence 保留给旧调用路径 fallback。新主路径使用
	// SessionEvidenceFrozen / SessionEvidenceOpen 两个一级字段。
	SessionEvidence string
	// TodoSnapshot 是会话级 TODO 列表渲染结果 (含 <|TODO_LIST_<nonce>|>...
	// 边界标签的整段块). 物理位置紧跟 SessionEvidence, 与 SessionEvidence
	// 一样落在 timeline-open 段, 不被 AI_CACHE_FROZEN / AI_CACHE_SEMI 任何
	// 缓存边界包裹, 避免污染上游 prefix cache.
	//
	// 关键词: TodoSnapshot, 全局 TODO 块, timeline-open 段位
	TodoSnapshot      string
	UserHistory       string
	FrozenUserContext string

	// ReportedRisks is the rendered "已报告漏洞清单" block injected at the
	// very end of the timeline-open prompt section (after PlanContext).
	// It lists all risks reported via cybersecurity-risk during this
	// session, so the model can avoid duplicate reports.
	//
	// 关键词: ReportedRisks, 已报告漏洞清单, timeline-open 末尾, 去重
	ReportedRisks string
}

// HighStaticData only selects between two stable protocol prefixes.
func (m *PromptMaterials) HighStaticData() map[string]any {
	return map[string]any{"FunctionCallMode": m != nil && m.FunctionCallMode}
}

// SemiDynamicData 供 caller-specific semi-dynamic 模板消费。
func (m *PromptMaterials) SemiDynamicData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"WorkspaceContext":     m.WorkspaceContext(),
		"SkillsContext":        m.SkillsContext,
		"PromotedSemiDynamic1": m.PromotedSemiDynamic1,
		"PlanHelp":             m.PlanHelp,
		"OriginalUserInput":    m.OriginalUserInput,
		"StableInstruction":    m.StableInstruction,
	}
}

// SemiDynamic1Data 兼容 aireact P1.1 命名。
func (m *PromptMaterials) SemiDynamic1Data() map[string]any {
	return m.SemiDynamicData()
}

// SemiDynamic2Data selects either the text example/schema or native action
// tags for this request. AutoLoadedSkills remain visible in both modes.
func (m *PromptMaterials) SemiDynamic2Data() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"FunctionCallMode":    m.FunctionCallMode,
		"TaskInstruction":     m.TaskInstruction,
		"ExecutionPolicy":     m.ExecutionPolicy,
		"Schema":              m.Schema,
		"FunctionCallSchemas": m.FunctionCallSchemas,
		"OutputExample":       m.OutputExample,
		"AutoLoadedSkills":    m.AutoLoadedSkills,
	}
}

// FrozenBlockData 供 frozen-block 模板消费, 顶部前置 ForcedSkills (用户强制加载 SKILL 满内容).
func (m *PromptMaterials) FrozenBlockData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"ForcedSkills":           m.ForcedSkills,
		"ToolInventory":          m.ToolInventory,
		"ToolsCount":             m.ToolsCount,
		"TopToolsCount":          m.TopToolsCount,
		"TopTools":               m.TopTools,
		"HasMoreTools":           m.HasMoreTools,
		"MoreToolsCount":         m.MoreToolsCount,
		"ForgeInventory":         m.ForgeInventory,
		"AIForgeList":            m.AIForgeList,
		"FrozenPartitions":       NormalizeFrozenBlockPartitions(m.FrozenPartitions),
		"SessionEvidenceFrozen":  m.SessionEvidenceFrozen,
		"TimelineFrozen":         m.TimelineFrozen,
		"TimelineFrozenTimeUnix": m.TimelineFrozenTimeUnix,
	}
}

// TimelineOpenData supplies the variable tail. The main ReAct loop places
// workspace coordinates in SemiDynamic1 and clears CurrentTime here before
// rendering, then emits the clock in Dynamic. Other callers retain their
// existing clock behavior.
func (m *PromptMaterials) TimelineOpenData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	sessionEvidenceOpen := m.SessionEvidenceOpen
	if sessionEvidenceOpen == "" {
		sessionEvidenceOpen = m.SessionEvidence
	}
	return map[string]any{
		"TimelineOpen":           m.TimelineOpen,
		"PromotedTimelineOpen":   m.PromotedTimelineOpen,
		"TimelineFrozenTimeUnix": m.TimelineFrozenTimeUnix,
		"SessionEvidence":        sessionEvidenceOpen,
		"TodoSnapshot":           m.TodoSnapshot,
		"Workspace":              m.Workspace,
		"OSArch":                 m.OSArch,
		"WorkingDir":             m.WorkingDir,
		"WorkingDirGlance":       m.WorkingDirGlance,
		"UserHistory":            m.UserHistory,
		"CurrentTime":            m.CurrentTime,
		"PlanContext":            m.FrozenUserContext,
		"ReportedRisks":          m.ReportedRisks,
	}
}

type TimelineFrozenOpenBlocks struct {
	Frozen               string
	Open                 string
	PromotedOpen         string
	PromotedSemiDynamic1 string
	FrozenTimeUnix       int64
}

func RenderTimelineFrozenOpen(timeline *Timeline) TimelineFrozenOpenBlocks {
	return renderTimelineFrozenOpen(timeline, false)
}

// RenderTimelineFrozenOpenWithLatestModelReplay is reserved for the main ReAct
// decision prompt. Helper prompts use RenderTimelineFrozenOpen and therefore
// never receive an internal replay marker.
func RenderTimelineFrozenOpenWithLatestModelReplay(timeline *Timeline) TimelineFrozenOpenBlocks {
	return renderTimelineFrozenOpen(timeline, true)
}

func renderTimelineFrozenOpen(timeline *Timeline, includeLatestModelReplay bool) TimelineFrozenOpenBlocks {
	if timeline == nil {
		return TimelineFrozenOpenBlocks{}
	}
	rb := timeline.GroupByMinutes(TimelineDumpDefaultIntervalMinutes).GetAllRenderable()
	var sealedBeforeID int64
	for _, block := range rb {
		interval, ok := block.(*TimelineIntervalBlock)
		if !ok || interval == nil || !interval.Open || len(interval.Items) == 0 {
			continue
		}
		sealedBeforeID = interval.Items[0].GetID()
		break
	}
	promotedSemi1, openDeltas := timeline.projectPromoted(sealedBeforeID)
	promptBlocks := projectTimelineRenderableBlocksForPrompt(rb)
	if includeLatestModelReplay {
		promptBlocks = projectTimelineRenderableBlocksForPromptWithLatestModelReplay(rb)
	}
	return TimelineFrozenOpenBlocks{
		Frozen:               promptBlocks.RenderFrozenOnly(TimelineDumpDefaultAITagName),
		Open:                 promptBlocks.RenderOpenOnly(TimelineDumpDefaultAITagName),
		PromotedOpen:         openDeltas,
		PromotedSemiDynamic1: promotedSemi1,
		FrozenTimeUnix:       timelineFrozenTimeUnixFromRenderable(rb),
	}
}

type PromptFrozenOpenMaterials struct {
	TimelineFrozen         string
	TimelineOpen           string
	PromotedTimelineOpen   string
	PromotedSemiDynamic1   string
	TimelineFrozenTimeUnix int64
	FrozenPartitions       []FrozenBlockPartition
	// Deprecated compatibility fields. Prompt construction no longer scans or
	// renders Session Artifacts.
	SessionArtifactsFrozen string
	SessionArtifactsOpen   string

	SessionEvidenceFrozen string
	SessionEvidenceOpen   string

	// ReportedRisks is the rendered "已报告漏洞清单" block for the
	// timeline-open section. Populated from SessionPromptState.
	ReportedRisks string
}

func BuildPromptFrozenOpenMaterials(config *Config, openNonce ...string) PromptFrozenOpenMaterials {
	return buildPromptFrozenOpenMaterials(config, false, openNonce...)
}

// BuildPromptFrozenOpenMaterialsWithLatestModelReplay is the main ReAct
// counterpart of BuildPromptFrozenOpenMaterials. Keeping this opt-in prevents
// LiteForge, verification, summarizers and other shared prompt builders from
// replaying a decision that belongs to the ReAct action protocol.
func BuildPromptFrozenOpenMaterialsWithLatestModelReplay(config *Config, openNonce ...string) PromptFrozenOpenMaterials {
	return buildPromptFrozenOpenMaterials(config, true, openNonce...)
}

func buildPromptFrozenOpenMaterials(config *Config, includeLatestModelReplay bool, openNonce ...string) PromptFrozenOpenMaterials {
	if config == nil {
		return PromptFrozenOpenMaterials{}
	}
	nonce := ""
	if len(openNonce) > 0 {
		nonce = openNonce[0]
	}
	timelineBlocks := RenderTimelineFrozenOpen(config.GetTimeline())
	if includeLatestModelReplay {
		timelineBlocks = RenderTimelineFrozenOpenWithLatestModelReplay(config.GetTimeline())
	}
	evidenceBlocks := config.GetSessionPromptState().GetSessionEvidenceFrozenOpenBlocks(timelineBlocks.FrozenTimeUnix, nonce)
	reportedRisks := config.GetSessionPromptState().GetReportedRisksRendered()
	return PromptFrozenOpenMaterials{
		TimelineFrozen:         timelineBlocks.Frozen,
		TimelineOpen:           timelineBlocks.Open,
		PromotedTimelineOpen:   timelineBlocks.PromotedOpen,
		PromotedSemiDynamic1:   timelineBlocks.PromotedSemiDynamic1,
		TimelineFrozenTimeUnix: timelineBlocks.FrozenTimeUnix,
		FrozenPartitions:       FrozenBlockPartitionsFromConfig(config),
		SessionEvidenceFrozen:  evidenceBlocks.Frozen,
		SessionEvidenceOpen:    evidenceBlocks.Open,
		ReportedRisks:          reportedRisks,
	}
}

func ApplyPromptFrozenOpenMaterials(materials *PromptMaterials, frozenOpen PromptFrozenOpenMaterials) {
	if materials == nil {
		return
	}
	materials.TimelineFrozen = frozenOpen.TimelineFrozen
	materials.TimelineOpen = frozenOpen.TimelineOpen
	materials.PromotedTimelineOpen = frozenOpen.PromotedTimelineOpen
	materials.PromotedSemiDynamic1 = frozenOpen.PromotedSemiDynamic1
	materials.TimelineFrozenTimeUnix = frozenOpen.TimelineFrozenTimeUnix
	materials.FrozenPartitions = append([]FrozenBlockPartition(nil), NormalizeFrozenBlockPartitions(frozenOpen.FrozenPartitions)...)
	materials.SessionEvidenceFrozen = frozenOpen.SessionEvidenceFrozen
	materials.SessionEvidenceOpen = frozenOpen.SessionEvidenceOpen
	materials.ReportedRisks = frozenOpen.ReportedRisks
}

func timelineFrozenTimeUnixFromRenderable(blocks TimelineRenderableBlocks) int64 {
	if len(blocks) == 0 {
		return 0
	}
	var lastFrozenEnd int64
	for _, block := range blocks {
		if block == nil {
			continue
		}
		interval, ok := block.(*TimelineIntervalBlock)
		if !ok || interval == nil {
			continue
		}
		if block.IsOpen() {
			if !interval.BucketStart.IsZero() {
				return interval.BucketStart.Unix()
			}
			return 0
		}
		if !interval.BucketEnd.IsZero() {
			lastFrozenEnd = interval.BucketEnd.Unix()
		}
	}
	return lastFrozenEnd
}
