package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

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
	//   - SkillsContext: Timeline 中已冻结并提升的技能正文
	//   - aid: PlanHelp / OriginalUserInput / StableInstruction 等 prompt-specific
	//     但仍属可缓存半动态前缀的内容
	SkillsContext string
	// PromotedRecentTools contains frozen tool descriptions and direct-call schemas.
	PromotedRecentTools string
	// PromotedUserInputHistory contains exact user inputs from frozen Timeline records.
	PromotedUserInputHistory string
	// PlanDocument contains the confirmed plan document rendered in main-loop SemiDynamic1.
	PlanDocument string
	// PlanDefinition is the current task tree and executable DAG,
	// without worker status or results. Only native coordinator sessions supply it.
	PlanDefinition string
	// CurrentReport is the editable report view in SemiDynamic1, never Frozen.
	CurrentReport     string
	PlanHelp          string
	OriginalUserInput string
	StableInstruction string

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
	// SkillInventory is the bounded directory fixed when the skill manager is initialized.
	// Loaded skill content still comes exclusively from Timeline.
	SkillInventory string
	ForgeInventory bool
	AIForgeList    string

	TimelineFrozen             string
	TimelineOpen               string
	TimelineFrozenTimeUnix     int64
	FrozenPartitions           []FrozenBlockPartition
	SessionEvidenceSemiDynamic string
	CurrentTime                string
	Workspace                  bool
	OSArch                     string
	WorkingDir                 string
	AIArtifactsDir             string

	// PlanStatus is the read-only execution snapshot immediately before TODO.
	PlanStatus string
	// TodoSnapshot 是会话级 TODO 列表渲染结果 (含 <|TODO_LIST_<nonce>|>...
	// 边界标签的整段块). 物理位置在普通 Timeline 之后，落在 timeline-open 段, 不被 AI_CACHE_FROZEN / AI_CACHE_SEMI 任何
	// 缓存边界包裹, 避免污染上游 prefix cache.
	//
	// 关键词: TodoSnapshot, 全局 TODO 块, timeline-open 段位
	TodoSnapshot string
	// Legacy helper prompts use this bounded history; main loops use Timeline.
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

// SemiDynamicData 供 caller-specific semi-dynamic 模板消费。
func (m *PromptMaterials) SemiDynamicData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"WorkspaceContext":           m.WorkspaceContext(),
		"SkillsContext":              m.SkillsContext,
		"PromotedRecentTools":        m.PromotedRecentTools,
		"PromotedUserInputHistory":   m.PromotedUserInputHistory,
		"PlanDocument":               m.PlanDocument,
		"PlanDefinition":             m.PlanDefinition,
		"CurrentReport":              m.CurrentReport,
		"SessionEvidenceSemiDynamic": m.SessionEvidenceSemiDynamic,
		"PlanHelp":                   m.PlanHelp,
		"OriginalUserInput":          m.OriginalUserInput,
		"StableInstruction":          m.StableInstruction,
	}
}

// SemiDynamic1Data 兼容 aireact P1.1 命名。
func (m *PromptMaterials) SemiDynamic1Data() map[string]any {
	return m.SemiDynamicData()
}

// SemiDynamic2Data selects either the text example/schema or native action
// tags for this request.
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
	}
}

// FrozenBlockData 供 frozen-block 模板消费。技能正文不再进入 frozen block:
// 它们统一在 SemiDynamic1 的单个技能块渲染; 未冻结部分留在 Timeline Open。
func (m *PromptMaterials) FrozenBlockData() map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return map[string]any{
		"FunctionCallMode": m.FunctionCallMode,
		"ToolInventory":    m.ToolInventory,
		"ToolsCount":       m.ToolsCount,
		"TopToolsCount":    m.TopToolsCount,
		"TopTools":         m.TopTools,
		"HasMoreTools":     m.HasMoreTools,
		"MoreToolsCount":   m.MoreToolsCount,
		"SkillInventory":   m.SkillInventory,
		"ForgeInventory":   m.ForgeInventory,
		"AIForgeList":      m.AIForgeList,
		"FrozenPartitions": NormalizeFrozenBlockPartitions(m.FrozenPartitions),

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
	return map[string]any{
		"TimelineOpen":           m.TimelineOpen,
		"TimelineFrozenTimeUnix": m.TimelineFrozenTimeUnix,
		"PlanStatus":             m.PlanStatus,
		"TodoSnapshot":           m.TodoSnapshot,
		"UserHistory":            m.UserHistory,
		"CurrentTime":            m.CurrentTime,
		"PlanContext":            m.FrozenUserContext,
		"ReportedRisks":          m.ReportedRisks,
	}
}

type TimelineFrozenOpenBlocks struct {
	ThroughID                int64
	ItemIDs                  []int64
	Frozen                   string
	Open                     string
	SkillsContext            string
	PromotedRecentTools      string
	PromotedUserInputHistory string
	FrozenTimeUnix           int64
	EvidenceSemiDynamic      string
}

// PromptText combines only the Timeline-owned material supplied to the prompt
// templates, in section order. Threshold checks use these same rendered fields,
// not journal JSON, compression-request text, or an extra effective-state fold.
func (b TimelineFrozenOpenBlocks) PromptText() string {
	var parts []string
	for _, text := range []string{b.Frozen, b.SkillsContext, b.PromotedRecentTools,
		b.PromotedUserInputHistory, b.EvidenceSemiDynamic, b.Open} {
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func RenderTimelineFrozenOpen(timeline *Timeline) TimelineFrozenOpenBlocks {
	return RenderTimelineFrozenOpenWithOptions(timeline, TimelinePromptOptions{})
}

// RenderTimelineFrozenOpenWithLatestModelReplay is reserved for the main ReAct
// decision prompt. Helper prompts use RenderTimelineFrozenOpen and therefore
// never receive an internal replay marker.
func RenderTimelineFrozenOpenWithLatestModelReplay(timeline *Timeline) TimelineFrozenOpenBlocks {
	return RenderTimelineFrozenOpenWithOptions(timeline, TimelinePromptOptions{IncludeLatestModelReplay: true})
}

// TimelinePromptOptions selects a read-only view, never new freeze boundaries.
// ExcludeToolCache omits structured cache events and their semi snapshot, not
// ordinary history mentioning tools or containing tool execution results.
type TimelinePromptOptions struct {
	IncludeLatestModelReplay bool
	ExcludeToolCache         bool
	// PromoteUserInput is opt-in for main context construction. Helper and
	// parameter prompts retain typed user records in their ordinary history.
	PromoteUserInput bool
	// UserInputOnly supplies exact user context, skill guidance and session evidence to
	// lightweight loops; ordinary execution history uses a separate recent window.
	UserInputOnly bool
	// ThroughID caps only the Open tail. Nil renders the current timeline.
	// Frozen and semi-dynamic sections are owned by compression.
	ThroughID *int64
}

func (timeline *Timeline) promptThroughLocked(options TimelinePromptOptions) (through int64, limited bool) {
	through = timeline.getMaxIDLocked()
	if options.ThroughID == nil || *options.ThroughID < 0 {
		return through, false
	}
	return min(*options.ThroughID, through), true
}

// capOpenTimelineBlocks caps only Open intervals. Frozen intervals and the
// compressed head are compression-owned state and remain unchanged.
func capOpenTimelineBlocks(blocks TimelineRenderableBlocks, throughID int64) TimelineRenderableBlocks {
	out := make(TimelineRenderableBlocks, 0, len(blocks))
	for _, block := range blocks {
		interval, ok := block.(*TimelineIntervalBlock)
		if !ok || !interval.Open {
			out = append(out, block)
			continue
		}
		// An empty Open interval may be retained to preserve a filtered tool-cache
		// boundary. Keep that boundary; only cap intervals that have real items.
		if len(interval.Items) == 0 {
			out = append(out, block)
			continue
		}
		copy := *interval
		copy.Items = nil
		for _, item := range interval.Items {
			if item != nil && item.GetID() <= throughID {
				copy.Items = append(copy.Items, item)
			}
		}
		if len(copy.Items) > 0 {
			out = append(out, &copy)
		}
	}
	return out
}

func RenderTimelineFrozenOpenWithOptions(timeline *Timeline, options TimelinePromptOptions) TimelineFrozenOpenBlocks {
	if timeline == nil {
		return TimelineFrozenOpenBlocks{}
	}
	// Freeze is committed on writes/import, never as a rendering side effect.
	timeline.mu.RLock()
	defer timeline.mu.RUnlock()
	return timeline.renderTimelinePromptLocked(options)
}

// Both prompt assembly and compression accounting capture these fields under
// one lock, so Open and promoted content cannot be omitted or counted twice
// while moving across a freeze boundary.
func (timeline *Timeline) renderTimelinePromptLocked(options TimelinePromptOptions) TimelineFrozenOpenBlocks {
	// Resolve the optional Open-tail boundary first. Frozen and semi-dynamic
	// content are independent of this cap.
	through, limited := timeline.promptThroughLocked(options)

	// Build the complete block view, then cap only Open intervals before
	// prompt projection.
	rawBlocks := timeline.promptBlocksLocked(options.ExcludeToolCache, options.UserInputOnly, options.PromoteUserInput || options.UserInputOnly)
	if limited {
		rawBlocks = capOpenTimelineBlocks(rawBlocks, through)
	}
	promptBlocks := projectTimelineRenderableBlocksForPrompt(rawBlocks)
	if options.IncludeLatestModelReplay {
		promptBlocks = projectTimelineRenderableBlocksForPromptWithLatestModelReplay(rawBlocks)
	}
	if options.ExcludeToolCache {
		visible := make(TimelineRenderableBlocks, 0, len(promptBlocks))
		for _, block := range promptBlocks {
			if interval, ok := block.(*TimelineIntervalBlock); ok && len(interval.Items) == 0 {
				continue
			}
			visible = append(visible, block)
		}
		promptBlocks = visible
	}

	// Semi-dynamic sections are stable snapshots and never capped by ThroughID.
	promoted := timeline.promotedState
	var recentTools string
	if !options.ExcludeToolCache && !options.UserInputOnly {
		recentTools = renderPromotedRecentTools(promoted)
	}
	var userInputHistory string
	if options.PromoteUserInput || options.UserInputOnly {
		userInputHistory = timeline.projectUserInputHistoryLocked()
	}

	// Compression candidates share the same Open boundary as the rendered tail.
	itemIDs := timeline.compressionItemIDsLocked(through)

	return TimelineFrozenOpenBlocks{
		ThroughID:                through,
		ItemIDs:                  itemIDs,
		Frozen:                   promptBlocks.RenderFrozenOnly(TimelineDumpDefaultAITagName),
		Open:                     promptBlocks.RenderOpenOnly(TimelineDumpDefaultAITagName),
		SkillsContext:            renderPromotedSkillContent(promoted),
		PromotedRecentTools:      recentTools,
		PromotedUserInputHistory: userInputHistory,
		FrozenTimeUnix:           timelineFrozenTimeUnixFromRenderable(rawBlocks),
		EvidenceSemiDynamic:      timeline.projectEvidenceLocked(),
	}
}

type PromptFrozenOpenMaterials struct {
	TimelineThroughID          int64
	TimelineItemIDs            []int64
	TimelineFrozen             string
	TimelineOpen               string
	SkillsContext              string
	PromotedRecentTools        string
	PromotedUserInputHistory   string
	TimelineFrozenTimeUnix     int64
	FrozenPartitions           []FrozenBlockPartition
	SessionEvidenceSemiDynamic string

	// ReportedRisks is the rendered "已报告漏洞清单" block for the
	// timeline-open section. Populated from SessionPromptState.
	ReportedRisks string
}

func BuildPromptFrozenOpenMaterials(config *Config, openNonce ...string) PromptFrozenOpenMaterials {
	return BuildPromptFrozenOpenMaterialsWithOptions(config, TimelinePromptOptions{})
}

// BuildPromptFrozenOpenMaterialsWithLatestModelReplay is the main ReAct
// counterpart of BuildPromptFrozenOpenMaterials. Keeping this opt-in prevents
// LiteForge, verification, summarizers and other shared prompt builders from
// replaying a decision that belongs to the ReAct action protocol.
func BuildPromptFrozenOpenMaterialsWithLatestModelReplay(config *Config, openNonce ...string) PromptFrozenOpenMaterials {
	return BuildPromptFrozenOpenMaterialsWithOptions(config, TimelinePromptOptions{IncludeLatestModelReplay: true})
}

// BuildPromptFrozenOpenMaterialsWithOptions applies the same typed filter to Open and Semi.
func BuildPromptFrozenOpenMaterialsWithOptions(config *Config, options TimelinePromptOptions) PromptFrozenOpenMaterials {
	if config == nil {
		return PromptFrozenOpenMaterials{}
	}
	timelineBlocks := RenderTimelineFrozenOpenWithOptions(config.GetTimeline(), options)
	reportedRisks := config.GetSessionPromptState().GetReportedRisksRendered()
	return PromptFrozenOpenMaterials{
		TimelineThroughID:          timelineBlocks.ThroughID,
		TimelineItemIDs:            timelineBlocks.ItemIDs,
		TimelineFrozen:             timelineBlocks.Frozen,
		TimelineOpen:               timelineBlocks.Open,
		SkillsContext:              timelineBlocks.SkillsContext,
		PromotedRecentTools:        timelineBlocks.PromotedRecentTools,
		PromotedUserInputHistory:   timelineBlocks.PromotedUserInputHistory,
		TimelineFrozenTimeUnix:     timelineBlocks.FrozenTimeUnix,
		FrozenPartitions:           FrozenBlockPartitionsFromConfig(config),
		SessionEvidenceSemiDynamic: timelineBlocks.EvidenceSemiDynamic,
		ReportedRisks:              reportedRisks,
	}
}

func ApplyPromptFrozenOpenMaterials(materials *PromptMaterials, frozenOpen PromptFrozenOpenMaterials) {
	if materials == nil {
		return
	}
	materials.TimelineFrozen = frozenOpen.TimelineFrozen
	materials.TimelineOpen = frozenOpen.TimelineOpen
	materials.SkillsContext = frozenOpen.SkillsContext
	materials.PromotedRecentTools = frozenOpen.PromotedRecentTools
	materials.PromotedUserInputHistory = frozenOpen.PromotedUserInputHistory
	materials.TimelineFrozenTimeUnix = frozenOpen.TimelineFrozenTimeUnix
	materials.FrozenPartitions = append([]FrozenBlockPartition(nil), NormalizeFrozenBlockPartitions(frozenOpen.FrozenPartitions)...)
	materials.SessionEvidenceSemiDynamic = frozenOpen.SessionEvidenceSemiDynamic
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
