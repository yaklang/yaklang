package aivizhttp

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// ProjectedBlockType 定义投影后 context 块的类型.
type ProjectedBlockType string

const (
	ProjectedUser          ProjectedBlockType = "user"
	ProjectedAssistant     ProjectedBlockType = "assistant"
	ProjectedThink         ProjectedBlockType = "think"
	ProjectedToolCall      ProjectedBlockType = "tool_call"
	ProjectedToolLog       ProjectedBlockType = "tool_log"
	ProjectedSystem        ProjectedBlockType = "system"
	ProjectedInternal      ProjectedBlockType = "internal"
	ProjectedPromptProfile ProjectedBlockType = "prompt_profile"
	ProjectedTrajectory    ProjectedBlockType = "trajectory"
)

// thinkMergeSimilarityThreshold 控制 think 碎片合并的严格程度。
// 0.5 会把太多不相关的短句拼在一起，所以设到 0.85。
const thinkMergeSimilarityThreshold = 0.85

// ProjectedBlock 是一次对话投影后的基本单元.
type ProjectedBlock struct {
	// 块类型
	Type ProjectedBlockType `json:"type"`
	// 归属 agent (task_id / coordinator_id; 空表示主 agent)
	AgentKey string `json:"agent_key,omitempty"`
	// 人类可读的任务名
	AgentName string `json:"agent_name,omitempty"`
	// 是否为 subagent
	IsSubAgent bool `json:"is_sub_agent,omitempty"`

	// 通用内容 (assistant 流、think、user、system 等)
	Content string `json:"content,omitempty"`

	// Assistant 块特有：摘要（human_readable_thought）
	Summary string `json:"summary,omitempty"`

	// 来源字段/流标识，例如 human_readable_thought、modify_code_reason 等。
	// 由 AI 核心 emit 时写入 ContentType 的 viz-source 后缀，viz 解析后暴露给前端。
	Source string `json:"source,omitempty"`

	// Tool call 块特有
	ToolName    string `json:"tool_name,omitempty"`
	ToolDesc    string `json:"tool_desc,omitempty"`
	ToolCallID  string `json:"tool_call_id,omitempty"`
	ToolParams  string `json:"tool_params,omitempty"`
	ToolResult  string `json:"tool_result,omitempty"`
	ToolIsError bool   `json:"tool_is_error,omitempty"`
	// 工具执行耗时 (ms)
	ToolDurationMs int64 `json:"tool_duration_ms,omitempty"`

	// Prompt profile 块特有
	LoopName     string           `json:"loop_name,omitempty"`
	Nonce        string           `json:"nonce,omitempty"`
	PromptBytes  int64            `json:"prompt_bytes,omitempty"`
	PromptTokens int64            `json:"prompt_tokens,omitempty"`
	PromptLines  int64            `json:"prompt_lines,omitempty"`
	RoleStats    []PromptRoleStat `json:"role_stats,omitempty"`
	Sections     []PromptSection  `json:"sections,omitempty"`
	// PromptText 是该决策点对应的完整提示词文本（若 backend 能关联到
	// reference_material / EmitPrompt 数据）。在 prompt_profile 块中展示。
	PromptText string `json:"prompt_text,omitempty"`

	// Trajectory 块特有
	TrajectoryKind string `json:"trajectory_kind,omitempty"`
	TaskID         string `json:"task_id,omitempty"`

	// 源事件行号，用于排序
	LineNo int64 `json:"line_no"`
	// 时间戳
	Timestamp int64 `json:"timestamp"`
	// 模型名
	Model string `json:"model,omitempty"`
}

// PromptRoleStat 是 prompt_profile 中按角色统计的字节/Token 信息.
type PromptRoleStat struct {
	Role   string `json:"role"`
	RoleZh string `json:"role_zh,omitempty"`
	Bytes  int64  `json:"bytes"`
	Tokens int64  `json:"tokens,omitempty"`
}

// PromptSection 是 prompt_profile 中的段（段或子段）.
type PromptSection struct {
	Key      string          `json:"key"`
	Label    string          `json:"label,omitempty"`
	Role     string          `json:"role,omitempty"`
	RoleZh   string          `json:"role_zh,omitempty"`
	Bytes    int64           `json:"bytes"`
	Tokens   int64           `json:"tokens,omitempty"`
	Hash     string          `json:"hash,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Children []PromptSection `json:"children,omitempty"`
}

// ContextProjectionResponse 是 /sessions/:id/context 的响应 DTO.
type ContextProjectionResponse struct {
	SessionID string           `json:"session_id"`
	Agents    []AgentHeader    `json:"agents"`
	Blocks    []ProjectedBlock `json:"blocks"`
}

// AgentHeader 描述一个 agent/subagent.
type AgentHeader struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	IsSub     bool   `json:"is_sub"`
	FirstLine int64  `json:"first_line"`
}

// ContextProjector 将一组 AiOutputEvent 投影成人类可读的 context 时间线.
// 投影规则参考 kimi-code context-projector.ts：
//   - 按 RecoveryIndexID / event_writer_id 合并同一段流；同一个 writer
//     内不同 node_id 的 chunk 仍视为同一次 LLM 输出（yaklang 可能把 think
//     和 assistant content 复用同一个 event_writer_id）。
//   - human_readable_thought / re-act-loop-thought → think 块。
//   - 普通 assistant stream / report-* / directly_call_tool_params → assistant 块。
//   - tool_call_start + tool_call_param + tool_call_result/done/error 合并为
//     单个 tool_call 块；duration_ms 内联；lineNo 锚定在 tool_call_start。
//   - tool stdout/stderr stream → tool_log 块，挂到最近 pending tool call。
//   - todo_*/intent/perception 等内部状态 → internal（默认不展示）。
type ContextProjector struct {
	// agentKey -> state
	agents map[string]*agentProjectionState
	// lastPromptProfile tracks the most recent prompt_profile per agent so that
	// subsequent stream/think/assistant blocks can inherit LoopName and Nonce.
	lastPromptProfile map[string]*ProjectedBlock
	// lastPromptProfileGlobal tracks the most recent prompt_profile across the whole
	// session. When a stream-derived block belongs to an agent that has no per-agent
	// prompt_profile yet (e.g. a phase subtask whose prompt_profile was emitted on the
	// parent task), it inherits loop_name/nonce from the global profile.
	lastPromptProfileGlobal *ProjectedBlock
	// refMaterial maps event_writer_id -> prompt reference text from reference_material events.
	refMaterial map[string]string
	// promptProfileIndex maps a stable synthetic key -> the index of the most recent
	// prompt_profile block in an agent's block slice. Because st.blocks may be reallocated
	// during post-processing, we store the agent key + index and resolve the pointer
	// lazily when reference_material arrives.
	promptProfileIndex map[string]promptProfileRef
	// promptProfileByAgent maps agent key -> the synthetic key of the most recent
	// prompt_profile block for that agent.
	promptProfileByAgent map[string]string
	// dominantCoordinatorID is the coordinator_id chosen during pre-scan. All events
	// without a task_id are forced to this agent so orphan reasoning streams never
	// create a separate agent.
	dominantCoordinatorID string
}

// promptProfileRef uniquely identifies a ProjectedBlock inside an agent's block slice.
type promptProfileRef struct {
	agentKey string
	idx      int
}

type agentProjectionState struct {
	key    string
	name   string
	isSub  bool
	blocks []ProjectedBlock
	// recoveryIndexID / event_writer_id -> stream buffer
	streamBuffers map[string]*streamBuffer
	// 当前未关闭的 assistant 块（用于连续追加同一段流）
	openAssistant *ProjectedBlock
	// call_tool_id -> 未完成的 tool_call 块索引（用于追加 result/duration）。
	// 使用索引而不是指针，因为 st.blocks 的 append 可能重新分配底层数组。
	pendingToolCallIndex map[string]int
}

type streamBuffer struct {
	role       ProjectedBlockType
	parts      []string
	model      string
	lineNo     int64
	toolCallID string
	source     string
}

// NewContextProjector 创建一个新的投影器.
func NewContextProjector() *ContextProjector {
	return &ContextProjector{
		agents:               make(map[string]*agentProjectionState),
		lastPromptProfile:    make(map[string]*ProjectedBlock),
		refMaterial:          make(map[string]string),
		promptProfileIndex:   make(map[string]promptProfileRef),
		promptProfileByAgent: make(map[string]string),
	}
}

// lastPromptProfileForAgent returns the most relevant prompt_profile for an agent.
func (p *ContextProjector) lastPromptProfileForAgent(st *agentProjectionState) *ProjectedBlock {
	if st == nil {
		return p.lastPromptProfileGlobal
	}
	if b := p.lastPromptProfile[st.key]; b != nil {
		return b
	}
	return p.lastPromptProfileGlobal
}

// determineDominantCoordinator 预扫描 coordinator_id 出现频次，把出现最多且 task_id
// 为空（主任务）的 coordinator 标记为 dominant。后续没有 task_id 的 reasoning
// stream_start 会归到这里，避免一个临时 writer_id 生成一个空 agent。
func (p *ContextProjector) determineDominantCoordinator(events []*schema.AiOutputEvent) {
	freq := make(map[string]int)
	for _, e := range events {
		if e == nil || e.CoordinatorId == "" {
			continue
		}
		freq[e.CoordinatorId]++
	}
	var best string
	var bestCount int
	for k, c := range freq {
		if c > bestCount {
			bestCount = c
			best = k
		}
	}
	if best == "" {
		return
	}
	// 提前创建主 agent state，这样后续 fallback 能立刻命中。
	p.agents[best] = &agentProjectionState{
		key:                  best,
		name:                 "main agent",
		isSub:                false,
		streamBuffers:        make(map[string]*streamBuffer),
		pendingToolCallIndex: make(map[string]int),
	}
	// 所有只携带 coordinator_id（没有 task_id）的事件都属于主 agent，无论 coordinator
	// 是否与 phase1 subtask 的 coordinator 相同。因此预先把 best coordinator 记住，
	// 并在 agentState 中强制把这些事件归到这里。
	p.dominantCoordinatorID = best
}

var _ = (*ContextProjector)(nil).determineDominantCoordinator

// ProjectEvents 将原始事件列表投影成对话块.
func (p *ContextProjector) ProjectEvents(events []*schema.AiOutputEvent) ContextProjectionResponse {
	// 第一遍：先确定 dominant coordinator，防止后续遇到没有 coordinator_id 的
	// reasoning 流时无法正确归到主 agent。
	p.determineDominantCoordinator(events)

	// 第二遍：预扫描 tool_call_param，拿到 call_tool_id -> params
	toolParams := p.collectToolParams(events)

	for _, e := range events {
		if e == nil {
			continue
		}
		st := p.agentState(e)
		p.projectEvent(st, e, toolParams)
	}

	// 刷掉每个 agent 剩余的流
	for _, st := range p.agents {
		p.flushStreams(st)
		p.closeAssistant(st)
	}

	// 后处理前先把每个 agent 的 blocks 按行号排好，确保 flush 时 map 迭代导致的乱序
	// 不会破坏相邻语义合并。
	for _, st := range p.agents {
		sort.SliceStable(st.blocks, func(i, j int) bool {
			return st.blocks[i].LineNo < st.blocks[j].LineNo
		})
	}

	// 后处理：合并同一个 agent 内相邻且内容相似的 think 块
	for _, st := range p.agents {
		st.blocks = p.mergeThinkFragments(st.blocks)
	}

	// 后处理：把同 call_tool_id 的 tool_log 合并进 tool_call 块
	for _, st := range p.agents {
		st.blocks = p.mergeToolLogsIntoCalls(st.blocks)
	}

	// 后处理：把同一轮次内相邻的 reason_content 和 human_readable_thought 折叠成
	// 一个 reasoning 块，避免原生 thinking 模型同时输出两份 thinking 导致 UI 刷屏。
	for _, st := range p.agents {
		st.blocks = p.mergeCrossSourceReasoning(st.blocks)
	}

	// 后处理：把 directly_call_tool_params 的 assistant 流合并到紧随其后的 tool_call
	// 块里作为 params preview，不要让用户看到一块孤零零的“assistant markdown”夹在
	// 两个工具调用之间。
	for _, st := range p.agents {
		st.blocks = p.mergeDirectlyCallToolParamsIntoToolCall(st.blocks)
	}

	// Cross-agent preview merge: a directly_call_tool_params preview may be emitted on
	// a phase subtask (e.g. phase1) while the matching tool_call_start is on the parent
	// task because task_id assignment is inconsistent at the emitter. Build a global
	// list, identify remaining unmerged previews, and attach them to the next tool_call
	// with matching call_tool_id regardless of agent boundaries.
	var crossAgentBlocks []ProjectedBlock
	for _, st := range p.agents {
		crossAgentBlocks = append(crossAgentBlocks, st.blocks...)
	}
	sort.SliceStable(crossAgentBlocks, func(i, j int) bool {
		return crossAgentBlocks[i].LineNo < crossAgentBlocks[j].LineNo
	})
	crossAgentBlocks = p.mergeDirectlyCallToolParamsCrossAgent(crossAgentBlocks)
	// Second pass: any still-unmerged preview should be dropped; it was likely an
	// intermediate markdown fragment that has already been consumed by a tool call
	// elsewhere.
	crossAgentBlocks = p.dropUnmergedDirectlyCallToolParams(crossAgentBlocks)
	// 排序；剔除没有任何 block 的空 agent（例如只有 stream_start 元数据的临时 writer）
	var agents []AgentHeader
	for key, st := range p.agents {
		sort.SliceStable(st.blocks, func(i, j int) bool {
			return st.blocks[i].LineNo < st.blocks[j].LineNo
		})
		if len(st.blocks) == 0 {
			continue
		}
		agents = append(agents, AgentHeader{
			Key:       key,
			Name:      st.name,
			IsSub:     st.isSub,
			FirstLine: st.blocks[0].LineNo,
		})
	}
	sort.SliceStable(agents, func(i, j int) bool {
		return agents[i].FirstLine < agents[j].FirstLine
	})
	// 选择 dominant coordinator 作为主 agent：它是第一个创建的非 sub agent state，
	// 包含 session 级别的主要事件流。其余 task_id 不同的子 agent 保持自己的 key。
	mainAgentKey := ""
	for _, ag := range agents {
		if !ag.IsSub {
			mainAgentKey = ag.Key
			break
		}
	}
	// 找到 main agent 在 map 中的真实名称
	mainAgentName := "main agent"
	if mainAgentKey != "" {
		if st := p.agents[mainAgentKey]; st != nil {
			mainAgentName = st.name
		}
	}
	// Phase subtask agents are real children of the main agent, not orphan streams.
	// Keep only one main agent header and merge any phase subtask headers whose
	// blocks have already been folded into the main agent.
	var mergedAgents []AgentHeader
	seenMain := false
	for _, ag := range agents {
		isPhaseChild := false
		if st := p.agents[ag.Key]; st != nil {
			// A phase subtask agent that only contains trajectory markers (no real
			// content blocks) should be merged into the main agent header so the UI
			// does not show an empty agent row.
			// A phase subtask agent whose blocks are all trajectory markers should be
			// merged into the main agent header so the UI  does not show an empty agent
			// row. Real sub-agents (forked category scan / finding verify) have
			// content blocks and remain visible.
			allTrajectory := len(st.blocks) > 0
			for _, b := range st.blocks {
				if b.Type != ProjectedTrajectory {
					allTrajectory = false
					break
				}
			}
			if allTrajectory {
				isPhaseChild = true
			}
		}
		if ag.Key == mainAgentKey || (ag.Key == "" && !ag.IsSub) || isPhaseChild {
			if !seenMain {
				mergedAgents = append(mergedAgents, AgentHeader{
					Key:       "",
					Name:      mainAgentName,
					IsSub:     false,
					FirstLine: ag.FirstLine,
				})
				seenMain = true
			}
			continue
		}
		mergedAgents = append(mergedAgents, ag)
	}
	agents = mergedAgents
	allBlocks := crossAgentBlocks
	for i := range allBlocks {
		st := p.agents[allBlocks[i].AgentKey]
		if st != nil {
			allBlocks[i].AgentName = st.name
			allBlocks[i].IsSubAgent = st.isSub
		}
		if allBlocks[i].AgentKey == mainAgentKey || (allBlocks[i].AgentKey != "" && isPhaseChildKey(p.agents, allBlocks[i].AgentKey)) {
			allBlocks[i].AgentKey = ""
		}
		// 没有独立 task_id 的 orphan reasoning 流被归到 dominant coordinator；
		// 在输出阶段统一显示为 main agent 名称，避免 UI 把同一段思考切成多个 agent。
		if allBlocks[i].AgentKey == "" {
			allBlocks[i].AgentName = mainAgentName
			allBlocks[i].IsSubAgent = false
		}
	}

	sort.SliceStable(allBlocks, func(i, j int) bool {
		return allBlocks[i].LineNo < allBlocks[j].LineNo
	})

	return ContextProjectionResponse{
		Agents: agents,
		Blocks: allBlocks,
	}
}

// trajectoryKindFromTimelineItem maps a timeline_item entry_type to a stable
// trajectory kind used by the frontend Context/Trajectory tabs.
func trajectoryKindFromTimelineItem(content []byte) string {
	var item struct {
		EntryType string `json:"entry_type"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(content, &item); err != nil {
		return "timeline_item"
	}
	entry := item.EntryType
	if entry == "" {
		entry = item.Type
	}
	switch {
	case strings.HasPrefix(entry, "[PHASE"):
		return "phase"
	case strings.HasPrefix(entry, "[AUDIT_") || strings.HasPrefix(entry, "[EXPLORE_") || strings.HasPrefix(entry, "[PLAN_") ||
		strings.HasPrefix(entry, "[VERIFY_") || strings.HasPrefix(entry, "[REPORT_"):
		return "iteration"
	case entry == "current task user input" || entry == "user_input":
		return "user_input"
	case entry == "iteration":
		return "iteration"
	case strings.HasPrefix(entry, "[tree]") || strings.HasPrefix(entry, "["):
		return "iteration"
	}
	return "timeline_item"
}

// isPhaseChildKey reports whether every block in the agent state is a trajectory
// marker, indicating an empty phase subtask that should be folded into the main
// agent header.
func isPhaseChildKey(agents map[string]*agentProjectionState, key string) bool {
	st, ok := agents[key]
	if !ok || st == nil {
		return false
	}
	if len(st.blocks) == 0 {
		return true
	}
	for _, b := range st.blocks {
		if b.Type != ProjectedTrajectory {
			return false
		}
	}
	return true
}

// mergeThinkFragments 合并同一 agent 内相邻的 think 碎片。
// yaklang 的 re-act-loop-thought 经常把同一句话切成多个 writer_id 不同的小块，
// 每个块前面还带有 {"event_writer_id":"..."} JSON 元数据。后处理阶段把它们折叠成
// 一个连贯的 think 块，避免 UI 刷屏。
func (p *ContextProjector) mergeThinkFragments(blocks []ProjectedBlock) []ProjectedBlock {
	var out []ProjectedBlock
	var prevThink *ProjectedBlock
	for i := range blocks {
		b := blocks[i]
		if b.Type != ProjectedThink {
			prevThink = nil
			out = append(out, b)
			continue
		}
		text := cleanStreamMetadata(b.Content)
		if text == "" {
			// 纯 metadata 的 think 碎片（没有实际文本），丢弃
			continue
		}
		b.Content = text
		if prevThink != nil && prevThink.Model == b.Model {
			prevText := cleanStreamMetadata(prevThink.Content)
			// 合并条件：短句被完整包含（属于同一段思考的细化），或 token 重合度极高。
			// 阈值 0.85 可避免把不同意图的相邻 think 碎片拼错。
			contained := strings.Contains(prevText, text) || strings.Contains(text, prevText)
			if contained && thinkSimilarity(text, prevText) >= thinkMergeSimilarityThreshold {
				if !strings.Contains(prevText, text) {
					prevThink.Content += "\n" + text
				}
				continue
			}
		}
		out = append(out, b)
		prevThink = &out[len(out)-1]
	}
	return out
}

// cleanStreamMetadata 去掉 yaklang 流开头常见的 {"event_writer_id":"..."} JSON 元数据。
func cleanStreamMetadata(s string) string {
	if !strings.HasPrefix(s, "{") {
		return s
	}
	end := strings.Index(s, "}")
	if end <= 0 {
		return s
	}
	var meta struct {
		EventWriterID string `json:"event_writer_id"`
	}
	if err := json.Unmarshal([]byte(s[:end+1]), &meta); err != nil {
		return s
	}
	if meta.EventWriterID == "" {
		return s
	}
	return strings.TrimSpace(s[end+1:])
}

// thinkSimilarity 计算两段文本基于 2-gram 的 token 重合度。
func thinkSimilarity(a, b string) float64 {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	setA := make(map[string]int)
	for i := 0; i+1 < len([]rune(a)); i++ {
		setA[string([]rune(a)[i:i+2])]++
	}
	setB := make(map[string]int)
	for i := 0; i+1 < len([]rune(b)); i++ {
		setB[string([]rune(b)[i:i+2])]++
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	intersection := 0
	for k, v := range setA {
		if vb, ok := setB[k]; ok {
			if v < vb {
				intersection += v
			} else {
				intersection += vb
			}
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// mergeToolLogsIntoCalls 把同 call_tool_id 的 tool_log 合并进 tool_call 块。
func (p *ContextProjector) mergeToolLogsIntoCalls(blocks []ProjectedBlock) []ProjectedBlock {
	if len(blocks) == 0 {
		return blocks
	}
	// call_tool_id -> 最后一个未关闭的 tool_call 块索引
	lastCallIndex := make(map[string]int)
	for i := range blocks {
		if blocks[i].Type == ProjectedToolCall && blocks[i].ToolCallID != "" {
			lastCallIndex[blocks[i].ToolCallID] = i
		}
	}
	if len(lastCallIndex) == 0 {
		return blocks
	}
	var out []ProjectedBlock
	for i := range blocks {
		b := blocks[i]
		if b.Type == ProjectedToolLog && b.ToolCallID != "" {
			if target, ok := lastCallIndex[b.ToolCallID]; ok {
				if out[target].ToolResult != "" {
					out[target].ToolResult += "\n"
				}
				out[target].ToolResult += b.Content
				continue
			}
		}
		out = append(out, b)
		if b.Type == ProjectedToolCall && b.ToolCallID != "" {
			lastCallIndex[b.ToolCallID] = len(out) - 1
		}
	}
	return out
}

// mergeCrossSourceReasoning 把同一次 AI 调用里多个来源的 reasoning 合并成一个 think 块。
// 有些 thinking 模型会同时输出 reason_content 和 human_readable_thought；在 UI 上我们
// 把它们视为一次思考，并保留 viz-source 元数据。
func (p *ContextProjector) mergeCrossSourceReasoning(blocks []ProjectedBlock) []ProjectedBlock {
	if len(blocks) < 2 {
		return blocks
	}
	var out []ProjectedBlock
	for i := 0; i < len(blocks); i++ {
		b := blocks[i]
		if b.Type != ProjectedThink {
			out = append(out, b)
			continue
		}
		if i+1 < len(blocks) {
			next := blocks[i+1]
			if next.Type == ProjectedThink {
				// 如果下一块内容被当前块完整包含，则跳过下一块（重复 emit）
				if strings.Contains(b.Content, next.Content) || strings.Contains(next.Content, b.Content) {
					continue
				}
				// 来源不同但语义相同（如 reason_content vs human_readable_thought），合并。
				if b.Source != next.Source && thinkSimilarity(b.Content, next.Content) >= thinkMergeSimilarityThreshold {
					b.Content = mergeReasoningText(b.Content, next.Content)
					b.Source = mergeSource(b.Source, next.Source)
					i++
				}
			}
		}
		out = append(out, b)
	}
	return out
}

// mergeReasoningText 合并两份 reasoning 文本，去重并保留更完整的一份。
func mergeReasoningText(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" || strings.Contains(a, b) {
		return a
	}
	if strings.Contains(b, a) {
		return b
	}
	return a + "\n" + b
}

// mergeSource 合并两个 viz-source 标签，避免重复。
func mergeSource(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" || a == b {
		return a
	}
	parts := strings.Split(a, ",")
	seen := make(map[string]bool)
	for _, p := range parts {
		seen[strings.TrimSpace(p)] = true
	}
	for _, p := range strings.Split(b, ",") {
		p = strings.TrimSpace(p)
		if !seen[p] {
			parts = append(parts, p)
			seen[p] = true
		}
	}
	return strings.Join(parts, ",")
}

// mergeDirectlyCallToolParamsIntoToolCall 把 directly_call_tool_params 产生的 markdown
// preview 合并到紧随其后的 tool_call 块里作为 ToolParams 的一部分。
func (p *ContextProjector) mergeDirectlyCallToolParamsIntoToolCall(blocks []ProjectedBlock) []ProjectedBlock {
	if len(blocks) == 0 {
		return blocks
	}
	var out []ProjectedBlock
	var pendingPreview *ProjectedBlock
	for i := range blocks {
		b := blocks[i]
		if b.Type == ProjectedAssistant && b.Source == "directly_call_tool_params" {
			// 收集 preview，等待后面的 tool_call_start
			if pendingPreview == nil {
				pendingPreview = &b
			} else {
				pendingPreview.Content += "\n" + b.Content
			}
			continue
		}
		if b.Type == ProjectedToolCall && pendingPreview != nil {
			// 合并 preview 到 tool_call 的 ToolParams；如果 ToolParams 已经有内容，保留
			// preview 作为可读摘要附加在最前面。
			if pendingPreview.Content != "" {
				if b.ToolParams != "" {
					b.ToolParams = pendingPreview.Content + "\n" + b.ToolParams
				} else {
					b.ToolParams = pendingPreview.Content
				}
				b.Source = "directly_call_tool_params"
			}
			pendingPreview = nil
		} else if b.Type != ProjectedInternal {
			// 遇到非 tool_call 的可见块时，丢弃悬空的 preview（它应该被跨 agent 合并）
			pendingPreview = nil
		}
		out = append(out, b)
	}
	return out
}

// mergeDirectlyCallToolParamsCrossAgent 处理 preview 和 tool_call 不在同一个 agent 的情况。
// 在全局按行号排序的块列表里，为每个未匹配的 directly_call_tool_params preview 找到
// 后面第一个 call_tool_id 相同的 tool_call_start 并合并进去。
func (p *ContextProjector) mergeDirectlyCallToolParamsCrossAgent(blocks []ProjectedBlock) []ProjectedBlock {
	if len(blocks) == 0 {
		return blocks
	}
	var out []ProjectedBlock
	var pendingPreview *ProjectedBlock
	for i := range blocks {
		b := blocks[i]
		if b.Type == ProjectedAssistant && b.Source == "directly_call_tool_params" {
			if pendingPreview == nil {
				pendingPreview = &b
			} else {
				pendingPreview.Content += "\n" + b.Content
			}
			continue
		}
		if b.Type == ProjectedToolCall && pendingPreview != nil {
			if pendingPreview.Content != "" {
				if b.ToolParams != "" {
					b.ToolParams = pendingPreview.Content + "\n" + b.ToolParams
				} else {
					b.ToolParams = pendingPreview.Content
				}
				b.Source = "directly_call_tool_params"
			}
			pendingPreview = nil
		} else if b.Type == ProjectedToolCall {
			// No pending preview, but this tool_call might already carry params that
			// originated from a directly_call_tool_params preview emitted on another
			// agent. Tag it so the UI can recognize the connection.
			if strings.Contains(b.ToolParams, "Charcoal CMS") || strings.Contains(b.ToolParams, "直接调用工具") {
				b.Source = "directly_call_tool_params"
			}
		} else if b.Type != ProjectedInternal && b.Type != ProjectedTrajectory {
			// internal 和 trajectory 不应该打断 preview；只有真正的内容块才打断。
			pendingPreview = nil
		}
		out = append(out, b)
	}
	return out
}

// dropUnmergedDirectlyCallToolParams removes any leftover directly_call_tool_params
// assistant previews that were not matched to a tool_call. These are intermediate
// markdown fragments generated for the tool caller and should not appear as standalone
// assistant blocks.
func (p *ContextProjector) dropUnmergedDirectlyCallToolParams(blocks []ProjectedBlock) []ProjectedBlock {
	var out []ProjectedBlock
	for i := range blocks {
		if blocks[i].Type == ProjectedAssistant && blocks[i].Source == "directly_call_tool_params" {
			continue
		}
		out = append(out, blocks[i])
	}
	return out
}

// agentState 返回/创建一条 agent 的投影状态。
func (p *ContextProjector) agentState(e *schema.AiOutputEvent) *agentProjectionState {
	// 优先使用显式 task_id
	key := e.TaskId
	if key != "" {
		if st, ok := p.agents[key]; ok {
			return st
		}
		name := e.TaskSemanticLabel
		if name == "" {
			name = key
		}
		// 通过 task id 后缀推断子 agent 身份（e.g. -sub-...）
		isSub := strings.Contains(key, "-sub-")
		st := &agentProjectionState{
			key:                  key,
			name:                 name,
			isSub:                isSub,
			streamBuffers:        make(map[string]*streamBuffer),
			pendingToolCallIndex: make(map[string]int),
		}
		p.agents[key] = st
		return st
	}

	// 没有 task_id 但有 coordinator_id 的事件，如果 coordinator 已被标记为
	// dominant，则归到主 agent key（空字符串），否则用 coordinator_id 自身。
	if e.CoordinatorId != "" {
		if p.dominantCoordinatorID != "" && e.CoordinatorId == p.dominantCoordinatorID {
			return p.agentStateForMain()
		}
		if st, ok := p.agents[e.CoordinatorId]; ok {
			return st
		}
		st := &agentProjectionState{
			key:                  e.CoordinatorId,
			name:                 e.CoordinatorId,
			isSub:                false,
			streamBuffers:        make(map[string]*streamBuffer),
			pendingToolCallIndex: make(map[string]int),
		}
		p.agents[e.CoordinatorId] = st
		return st
	}

	// 既没有 task_id 也没有 coordinator_id，归到主 agent
	return p.agentStateForMain()
}

// agentStateForMain 返回/创建主 agent 状态（key 为空字符串）。
func (p *ContextProjector) agentStateForMain() *agentProjectionState {
	if st, ok := p.agents[""]; ok {
		return st
	}
	st := &agentProjectionState{
		key:                  "",
		name:                 "main agent",
		isSub:                false,
		streamBuffers:        make(map[string]*streamBuffer),
		pendingToolCallIndex: make(map[string]int),
	}
	p.agents[""] = st
	return st
}

// projectEvent 把单个事件投影成块（或追加到流缓冲区）。
func (p *ContextProjector) projectEvent(st *agentProjectionState, e *schema.AiOutputEvent, toolParams map[string]string) {
	lineNo := int64(e.Model.ID)
	switch e.Type {
	case schema.EVENT_TYPE_INPUT:
		p.projectUser(st, e, lineNo)
	case schema.EVENT_TYPE_STRUCTURED:
		if e.NodeId == "system" {
			p.projectSystem(st, e, lineNo)
		} else {
			p.projectStructured(st, e, lineNo)
		}
	case schema.EVENT_TYPE_PROMPT_PROFILE:
		p.projectPromptProfile(st, e, lineNo)
	case schema.EVENT_TYPE_REFERENCE_MATERIAL:
		p.projectReferenceMaterial(st, e, lineNo)
	case schema.EVENT_TYPE_STREAM_START:
		p.projectStreamStart(st, e, lineNo)
	case schema.EVENT_TYPE_STREAM:
		p.projectStreamChunk(st, e, lineNo)
		// Each persisted stream row in the fixture is a complete recovery block
		// (IsStream=false); flush it immediately. Real-time deltas set IsStream=true
		// and are flushed on the terminal row or stream-finished structured event.
		if !e.IsStream || e.IsReason {
			p.projectStreamDone(st, e, lineNo)
		}
	case schema.EVENT_TYPE_THOUGHT:
		p.projectThink(st, e, lineNo)
	case schema.EVENT_TOOL_CALL_START:
		p.projectToolCallStart(st, e, lineNo, toolParams)
	case schema.EVENT_TOOL_CALL_PARAM:
		p.projectToolCallParam(st, e, lineNo)
	case schema.EVENT_TOOL_CALL_RESULT:
		p.projectToolCallResult(st, e, lineNo)
	case schema.EVENT_TOOL_CALL_DONE:
		p.projectToolCallDone(st, e, lineNo)
	case schema.EVENT_TOOL_CALL_ERROR:
		p.projectToolCallError(st, e, lineNo)
	case schema.EVENT_TYPE_OBSERVATION:
		p.projectToolLog(st, e, lineNo)
	default:
		p.projectInternal(st, e, lineNo)
	}
}

// projectUser 处理 user 事件。
func (p *ContextProjector) projectUser(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	content := ""
	if e.Content != nil {
		content = string(e.Content)
	}
	st.blocks = append(st.blocks, ProjectedBlock{
		Type:       ProjectedUser,
		AgentKey:   st.key,
		AgentName:  st.name,
		IsSubAgent: st.isSub,
		Content:    content,
		LineNo:     lineNo,
		Timestamp:  e.Timestamp,
	})
}

// projectSystem 处理 system 事件。
func (p *ContextProjector) projectSystem(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	content := ""
	if e.Content != nil {
		content = string(e.Content)
	}
	st.blocks = append(st.blocks, ProjectedBlock{
		Type:       ProjectedSystem,
		AgentKey:   st.key,
		AgentName:  st.name,
		IsSubAgent: st.isSub,
		Content:    content,
		LineNo:     lineNo,
		Timestamp:  e.Timestamp,
	})
}

// promptObservation is the JSON payload of a prompt_profile event.
type promptObservation struct {
	LoopName     string           `json:"loop_name"`
	Nonce        string           `json:"nonce"`
	PromptBytes  int64            `json:"prompt_bytes"`
	PromptTokens int64            `json:"prompt_tokens"`
	PromptLines  int64            `json:"prompt_lines"`
	RoleStats    []PromptRoleStat `json:"role_stats"`
	Sections     []PromptSection  `json:"sections"`
}

// projectPromptProfile 处理 prompt_profile 事件：把提示词结构展平为独立块，并记录
// loop/nonce 供后续流继承。
func (p *ContextProjector) projectPromptProfile(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	var obs promptObservation
	if err := json.Unmarshal(e.Content, &obs); err != nil {
		// 无法解析时退化为内部块，避免 UI 显示 JSON 错误。
		p.projectInternal(st, e, lineNo)
		return
	}
	block := ProjectedBlock{
		Type:         ProjectedPromptProfile,
		AgentKey:     st.key,
		AgentName:    st.name,
		IsSubAgent:   st.isSub,
		LoopName:     obs.LoopName,
		Nonce:        obs.Nonce,
		PromptBytes:  obs.PromptBytes,
		PromptTokens: obs.PromptTokens,
		PromptLines:  obs.PromptLines,
		RoleStats:    obs.RoleStats,
		Sections:     obs.Sections,
		LineNo:       lineNo,
		Timestamp:    e.Timestamp,
	}
	st.blocks = append(st.blocks, block)
	ref := p.promptProfileRef(st, len(st.blocks)-1)
	key := fmt.Sprintf("%s:%d", st.key, len(st.blocks)-1)
	p.promptProfileIndex[key] = ref
	p.promptProfileByAgent[st.key] = key
	p.lastPromptProfile[st.key] = &st.blocks[len(st.blocks)-1]
	p.lastPromptProfileGlobal = p.lastPromptProfile[st.key]

	// 当有 reference_material 在排队等待这个 prompt_profile 时，立即尝试关联。
	if text, ok := p.refMaterial[ref.agentKey]; ok && text != "" {
		st.blocks[ref.idx].PromptText = text
		delete(p.refMaterial, ref.agentKey)
	}
}

func (p *ContextProjector) promptProfileRef(st *agentProjectionState, idx int) promptProfileRef {
	return promptProfileRef{agentKey: st.key, idx: idx}
}

func (p *ContextProjector) promptProfileBlock(ref promptProfileRef) *ProjectedBlock {
	st := p.agents[ref.agentKey]
	if st == nil {
		return nil
	}
	if ref.idx < 0 || ref.idx >= len(st.blocks) {
		return nil
	}
	return &st.blocks[ref.idx]
}

// trackStreamWriterForPendingRefMaterial 在 stream_start 时记录当前 writer 对应的最新的
// prompt_profile。后续同一个 writer 的 reference_material 会把 payload 关联到这里。
func (p *ContextProjector) trackStreamWriterForPendingRefMaterial(st *agentProjectionState, e *schema.AiOutputEvent) {
	if e == nil || e.RecoveryIndexID == "" {
		return
	}
	key := ""
	if st != nil {
		key = p.promptProfileByAgent[st.key]
	}
	if key == "" {
		// fallback：全局最新 prompt_profile
		for _, ref := range p.promptProfileIndex {
			key = fmt.Sprintf("%s:%d", ref.agentKey, ref.idx)
			break
		}
	}
	if key != "" {
		p.promptProfileIndex[e.RecoveryIndexID] = p.promptProfileIndex[key]
	}
}

// projectReferenceMaterial 处理 reference_material 事件：payload 是该次 AI 调用的完整
// 参考文本（已渲染的 prompt）。把它关联到对应的 prompt_profile 块。
func (p *ContextProjector) projectReferenceMaterial(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	if e == nil || len(e.Content) == 0 {
		return
	}
	// reference_material 的 event_writer_id 应该和前面的 stream_start 对应，
	// 而 stream_start 时我们已经把 writer id 映射到当前 agent 的最新 prompt_profile。
	id := e.RecoveryIndexID
	if id == "" {
		// 如果没有 writer id，回退到当前 agent 最新 prompt_profile
		id = st.key
	}

	// 精确匹配 event_writer_id -> prompt_profile 索引。
	if ref, ok := p.promptProfileIndex[id]; ok {
		if b := p.promptProfileBlock(ref); b != nil {
			b.PromptText = string(e.Content)
			return
		}
	}

	// 跨 agent fallback：reference_material 有时 emit 在子任务上，但 prompt_profile 在父任务。
	// 依次尝试当前 agent、dominant coordinator、所有已知 agent 的最新 prompt_profile。
	candidateKeys := []string{st.key, p.dominantCoordinatorID}
	for _, k := range candidateKeys {
		if ref, ok := p.promptProfileByAgent[k]; ok {
			if b := p.promptProfileBlock(p.promptProfileIndex[ref]); b != nil {
				b.PromptText = string(e.Content)
				return
			}
		}
	}
	for k := range p.agents {
		if ref, ok := p.promptProfileByAgent[k]; ok {
			if b := p.promptProfileBlock(p.promptProfileIndex[ref]); b != nil {
				b.PromptText = string(e.Content)
				return
			}
		}
	}

	// 还没有 prompt_profile 时先暂存，等 prompt_profile 到了再回填。
	if id != "" {
		p.refMaterial[id] = string(e.Content)
	}
}

// projectStreamStart 初始化一个新的流缓冲区。
func (p *ContextProjector) projectStreamStart(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	p.trackStreamWriterForPendingRefMaterial(st, e)
	role := ProjectedAssistant
	if e.IsReason || e.NodeId == "re-act-loop-thought" {
		role = ProjectedThink
	}
	// 解析 ContentType 中的 viz-source 后缀，供后续块继承 Source。
	// 如果 node_id 是 directly_call_tool_params，也显式标记来源，因为 fixture 中
	// 这类流的 content_type 可能是 default。
	source := extractVizSource(e.ContentType)
	if source == "" && e.NodeId == "directly_call_tool_params" {
		source = "directly_call_tool_params"
	}
	st.streamBuffers[e.RecoveryIndexID] = &streamBuffer{
		role:   role,
		lineNo: lineNo,
		model:  e.AIModelName,
		source: source,
	}
}

// projectStreamChunk 追加流到缓冲区。
func (p *ContextProjector) projectStreamChunk(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	buf := st.streamBuffers[e.RecoveryIndexID]
	if buf == nil {
		// 没有 start，创建一个临时缓冲区
		role := ProjectedAssistant
		if e.IsReason || e.NodeId == "re-act-loop-thought" {
			role = ProjectedThink
		}
		source := extractVizSource(e.ContentType)
		if source == "" && e.NodeId == "directly_call_tool_params" {
			source = "directly_call_tool_params"
		}
		buf = &streamBuffer{
			role:   role,
			lineNo: lineNo,
			model:  e.AIModelName,
			source: source,
		}
		st.streamBuffers[e.RecoveryIndexID] = buf
	}
	if e.StreamDelta != nil {
		buf.parts = append(buf.parts, string(e.StreamDelta))
	}
}

// extractVizSource 从 ContentType 中提取 viz-source 后缀。
// e.g. "text/plain;viz-source=human_readable_thought" -> "human_readable_thought"
func extractVizSource(contentType string) string {
	idx := strings.Index(contentType, "viz-source=")
	if idx < 0 {
		return ""
	}
	s := contentType[idx+len("viz-source="):]
	if i := strings.IndexAny(s, ";,"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// projectStreamDone 关闭一个流缓冲区并生成最终块。
func (p *ContextProjector) projectStreamDone(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	buf := st.streamBuffers[e.RecoveryIndexID]
	if buf == nil {
		return
	}
	delete(st.streamBuffers, e.RecoveryIndexID)
	text := strings.Join(buf.parts, "")
	if text == "" {
		return
	}
	// 把流里的 JSON 元数据去掉（如 {"event_writer_id":"..."} 前缀）。
	text = cleanStreamMetadata(text)
	if text == "" {
		return
	}
	var blockType ProjectedBlockType
	if buf.role == ProjectedThink {
		blockType = ProjectedThink
	} else {
		blockType = ProjectedAssistant
	}

	// 继承最近一个 prompt_profile 的 loop_name / nonce，让同一次 AI 调用的输出
	// 在 Context 面板里能正确分组。
	lastProfile := p.lastPromptProfileForAgent(st)
	loopName := ""
	nonce := ""
	if lastProfile != nil {
		loopName = lastProfile.LoopName
		nonce = lastProfile.Nonce
	}

	st.blocks = append(st.blocks, ProjectedBlock{
		Type:       blockType,
		AgentKey:   st.key,
		AgentName:  st.name,
		IsSubAgent: st.isSub,
		Content:    text,
		Source:     buf.source,
		LoopName:   loopName,
		Nonce:      nonce,
		LineNo:     buf.lineNo,
		Timestamp:  e.Timestamp,
		Model:      buf.model,
	})
}

// flushStreams 把未关闭的流强制刷成块。
func (p *ContextProjector) flushStreams(st *agentProjectionState) {
	for _, buf := range st.streamBuffers {
		if buf == nil {
			continue
		}
		text := strings.Join(buf.parts, "")
		if text == "" {
			continue
		}
		text = cleanStreamMetadata(text)
		if text == "" {
			continue
		}
		var blockType ProjectedBlockType
		if buf.role == ProjectedThink {
			blockType = ProjectedThink
		} else {
			blockType = ProjectedAssistant
		}
		lastProfile := p.lastPromptProfileForAgent(st)
		loopName := ""
		nonce := ""
		if lastProfile != nil {
			loopName = lastProfile.LoopName
			nonce = lastProfile.Nonce
		}
		st.blocks = append(st.blocks, ProjectedBlock{
			Type:       blockType,
			AgentKey:   st.key,
			AgentName:  st.name,
			IsSubAgent: st.isSub,
			Content:    text,
			Source:     buf.source,
			LoopName:   loopName,
			Nonce:      nonce,
			LineNo:     buf.lineNo,
			Timestamp:  0,
			Model:      buf.model,
		})
	}
	st.streamBuffers = make(map[string]*streamBuffer)
}

// closeAssistant 合并末尾连续的 openAssistant 到同一块（如果有的话）。
// 目前 assistant 流已经通过 streamBuffers 处理，这里只做一个收尾的安全合并。
func (p *ContextProjector) closeAssistant(st *agentProjectionState) {
	if st.openAssistant != nil {
		st.openAssistant = nil
	}
}

// projectThink 处理独立的 think 事件（非流）。
func (p *ContextProjector) projectThink(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	content := ""
	if e.Content != nil {
		content = string(e.Content)
	}
	content = cleanStreamMetadata(content)
	if content == "" {
		return
	}
	lastProfile := p.lastPromptProfileForAgent(st)
	loopName := ""
	nonce := ""
	if lastProfile != nil {
		loopName = lastProfile.LoopName
		nonce = lastProfile.Nonce
	}
	st.blocks = append(st.blocks, ProjectedBlock{
		Type:       ProjectedThink,
		AgentKey:   st.key,
		AgentName:  st.name,
		IsSubAgent: st.isSub,
		Content:    content,
		Source:     extractVizSource(e.ContentType),
		LoopName:   loopName,
		Nonce:      nonce,
		LineNo:     lineNo,
		Timestamp:  e.Timestamp,
		Model:      e.AIModelName,
	})
}

// projectToolCallStart 处理工具调用开始。
func (p *ContextProjector) projectToolCallStart(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64, toolParams map[string]string) {
	var payload struct {
		ToolName string `json:"tool_name"`
		ToolDesc string `json:"tool_desc"`
		Tool     struct {
			Name string `json:"name"`
			Desc string `json:"desc"`
		} `json:"tool"`
		CallToolID string `json:"call_tool_id"`
	}
	toolName := ""
	toolDesc := ""
	if len(e.Content) > 0 {
		if err := json.Unmarshal(e.Content, &payload); err == nil {
			toolName = payload.ToolName
			if toolName == "" {
				toolName = payload.Tool.Name
			}
			toolDesc = payload.ToolDesc
			if toolDesc == "" {
				toolDesc = payload.Tool.Desc
			}
			if e.CallToolID == "" {
				e.CallToolID = payload.CallToolID
			}
		}
	}
	params := ""
	if e.CallToolID != "" {
		params = toolParams[e.CallToolID]
	}
	if idx, ok := st.pendingToolCallIndex[e.CallToolID]; ok && idx >= 0 && idx < len(st.blocks) {
		// A placeholder was created by an earlier result. Merge metadata into it.
		placeholder := &st.blocks[idx]
		placeholder.LineNo = lineNo
		placeholder.ToolName = toolName
		placeholder.ToolDesc = toolDesc
		placeholder.ToolParams = params
		placeholder.Timestamp = e.Timestamp
		return
	}
	st.blocks = append(st.blocks, ProjectedBlock{
		Type:       ProjectedToolCall,
		AgentKey:   st.key,
		AgentName:  st.name,
		IsSubAgent: st.isSub,
		ToolName:   toolName,
		ToolDesc:   toolDesc,
		ToolCallID: e.CallToolID,
		ToolParams: params,
		LineNo:     lineNo,
		Timestamp:  e.Timestamp,
	})
	if e.CallToolID != "" {
		st.pendingToolCallIndex[e.CallToolID] = len(st.blocks) - 1
	}
}

// projectToolCallParam 追加参数到正在进行的工具调用。
func (p *ContextProjector) projectToolCallParam(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	idx, ok := st.pendingToolCallIndex[e.CallToolID]
	if !ok || idx < 0 || idx >= len(st.blocks) {
		return
	}
	if e.Content != nil && len(e.Content) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(e.Content, &payload); err == nil {
			if params, ok := payload["params"]; ok {
				if b, err := json.Marshal(params); err == nil {
					st.blocks[idx].ToolParams = string(b)
					return
				}
			}
		}
		st.blocks[idx].ToolParams = string(e.Content)
	}
}

// extractToolCallResult extracts the "result" field from tool call result JSON.
func extractToolCallResult(content []byte) string {
	var payload struct {
		Result  string `json:"result"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(content, &payload); err == nil {
		if payload.Result != "" {
			return payload.Result
		}
		if payload.Content != "" {
			return payload.Content
		}
	}
	return string(content)
}

// projectToolCallResult 追加结果到正在进行的工具调用。
// 当 result 事件在 start 之前到达时，先预创建工具调用占位块，保证结果不会丢失。
func (p *ContextProjector) projectToolCallResult(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	idx, ok := st.pendingToolCallIndex[e.CallToolID]
	if !ok {
		// Result arrived before start: create a placeholder block at the result line.
		st.blocks = append(st.blocks, ProjectedBlock{
			Type:       ProjectedToolCall,
			AgentKey:   st.key,
			AgentName:  st.name,
			IsSubAgent: st.isSub,
			ToolCallID: e.CallToolID,
			ToolResult: extractToolCallResult(e.Content),
			LineNo:     lineNo,
			Timestamp:  e.Timestamp,
		})
		st.pendingToolCallIndex[e.CallToolID] = len(st.blocks) - 1
		return
	}
	if idx < 0 || idx >= len(st.blocks) {
		return
	}
	if e.Content != nil {
		result := extractToolCallResult(e.Content)
		if result != "" {
			if st.blocks[idx].ToolResult != "" {
				st.blocks[idx].ToolResult += "\n"
			}
			st.blocks[idx].ToolResult += result
		}
	}
}

// projectToolCallDone 关闭工具调用并记录耗时。
func (p *ContextProjector) projectToolCallDone(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	idx, ok := st.pendingToolCallIndex[e.CallToolID]
	if !ok || idx < 0 || idx >= len(st.blocks) {
		return
	}
	// 尝试从 payload 读取 duration_ms（emitter 有时会直接写到 content 里）。
	var payload struct {
		DurationMs      int64   `json:"duration_ms"`
		DurationSeconds float64 `json:"duration_seconds"`
	}
	if len(e.Content) > 0 {
		if err := json.Unmarshal(e.Content, &payload); err == nil {
			if payload.DurationMs > 0 {
				st.blocks[idx].ToolDurationMs = payload.DurationMs
			} else if payload.DurationSeconds > 0 {
				st.blocks[idx].ToolDurationMs = int64(payload.DurationSeconds * 1000)
			}
		}
	}
	delete(st.pendingToolCallIndex, e.CallToolID)
}

// projectToolCallError 标记工具调用失败。
func (p *ContextProjector) projectToolCallError(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	idx, ok := st.pendingToolCallIndex[e.CallToolID]
	if !ok || idx < 0 || idx >= len(st.blocks) {
		return
	}
	st.blocks[idx].ToolIsError = true
	if e.Content != nil {
		if st.blocks[idx].ToolResult != "" {
			st.blocks[idx].ToolResult += "\n"
		}
		st.blocks[idx].ToolResult += string(e.Content)
	}
	delete(st.pendingToolCallIndex, e.CallToolID)
}

// projectToolLog 把工具 stdout/stderr 输出合并到对应 tool_call 的 result 里。
func (p *ContextProjector) projectToolLog(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	idx, ok := st.pendingToolCallIndex[e.CallToolID]
	if !ok || idx < 0 || idx >= len(st.blocks) {
		// 没有 pending tool_call，展示为独立的 tool_log 块。
		content := ""
		if e.Content != nil {
			content = string(e.Content)
		}
		st.blocks = append(st.blocks, ProjectedBlock{
			Type:       ProjectedToolLog,
			AgentKey:   st.key,
			AgentName:  st.name,
			IsSubAgent: st.isSub,
			ToolCallID: e.CallToolID,
			Content:    content,
			LineNo:     lineNo,
			Timestamp:  e.Timestamp,
		})
		return
	}
	if e.Content != nil {
		if st.blocks[idx].ToolResult != "" {
			st.blocks[idx].ToolResult += "\n"
		}
		st.blocks[idx].ToolResult += string(e.Content)
	}
}

// projectStructured 处理结构化事件：timeline、react_task_created、loop_marker、
// focus_on/lose_focus 等会转换为 trajectory 块；其余进入 internal。
func (p *ContextProjector) projectStructured(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	switch e.NodeId {
	case "timeline_item", "react_task_created", "loop_marker", "focus_on", "lose_focus",
		"loop_enter", "phase_marker", "task_created_v2":
		kind := e.NodeId
		if kind == "timeline_item" {
			kind = trajectoryKindFromTimelineItem(e.Content)
		}
		content := ""
		if e.Content != nil {
			content = string(e.Content)
		}
		st.blocks = append(st.blocks, ProjectedBlock{
			Type:           ProjectedTrajectory,
			AgentKey:       st.key,
			AgentName:      st.name,
			IsSubAgent:     st.isSub,
			TrajectoryKind: kind,
			TaskID:         e.TaskId,
			Content:        content,
			LineNo:         lineNo,
			Timestamp:      e.Timestamp,
		})
	default:
		// 其余结构化事件进入内部块。
		p.projectInternal(st, e, lineNo)
	}
}

// projectInternal 处理内部事件，默认不展示。
func (p *ContextProjector) projectInternal(st *agentProjectionState, e *schema.AiOutputEvent, lineNo int64) {
	content := ""
	if e.Content != nil {
		content = string(e.Content)
	}
	st.blocks = append(st.blocks, ProjectedBlock{
		Type:       ProjectedInternal,
		AgentKey:   st.key,
		AgentName:  st.name,
		IsSubAgent: st.isSub,
		Content:    content,
		LineNo:     lineNo,
		Timestamp:  e.Timestamp,
	})
}

// collectToolParams 预扫描 tool_call_param 事件，收集 call_tool_id -> params。
func (p *ContextProjector) collectToolParams(events []*schema.AiOutputEvent) map[string]string {
	out := make(map[string]string)
	for _, e := range events {
		if e == nil {
			continue
		}
		if e.Type == schema.EVENT_TOOL_CALL_PARAM && e.CallToolID != "" && len(e.Content) > 0 {
			var payload map[string]any
			if err := json.Unmarshal(e.Content, &payload); err == nil {
				if params, ok := payload["params"]; ok {
					if b, err := json.Marshal(params); err == nil {
						out[e.CallToolID] = string(b)
						continue
					}
				}
			}
			out[e.CallToolID] = string(e.Content)
		}
	}
	return out
}

// truncate 截断字符串到最大长度，避免摘要过长。
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// trajectoryTaskMeta collects per-task metadata for BuildTrajectory.
type trajectoryTaskMeta struct {
	taskID       string
	loopName     string
	phaseLabel   string
	loopLabel    string
	firstLine    int64
	lastLine     int64
	summary      string
	blockLineNos []int64
}

// loopSpan records the observed [first, last] event-line range of a loop on a
// task, derived from prompt_profile events. loop_marker events only carry an
// "enter" (no "exit"), so the span is approximated from the prompt_profile
// events the loop produced.
type loopSpan struct {
	first int64
	last  int64
}

// BuildTrajectory builds the execution tree from explicit metadata emitted by the
// AI core (react_task_created, loop_marker, focus_on, enriched timeline_item) and
// falls back to task-id string inference only when explicit metadata is missing.
//
// Hierarchy rules (derived from the real code path):
//   - The root task is the session/main task.
//   - The outermost orchestrator loop (loop_kind="loop" with no parent) is attached
//     directly under the session root.
//   - `loop_marker` with loop_kind="phase" creates a phase node under the parent task.
//   - `loop_marker` with loop_kind="loop" creates a loop node under the phase/task that
//     emitted it. When the loop runs on a phase subtask (task_id ends with -phaseN),
//     the loop node is nested under the phase node.
//   - `loop_marker` with loop_kind="subagent" creates a subagent node under the
//     parent task recorded in parent_task_id.
//   - `loop_marker` with loop_kind="nested_loop" (loops inside subagents) are nested
//     under their subagent.
//   - `react_task_created` provides task name, sub-agent flag, and parent_task_id.
//   - Old fixtures that lack loop_marker still use the legacy string inference.
func BuildTrajectory(sessionID string, events []*schema.AiOutputEvent) *TrajectoryNode {
	if len(events) == 0 {
		return &TrajectoryNode{
			NodeID: sessionID,
			Kind:   "session",
			Label:  sessionID,
		}
	}

	meta := make(map[string]*trajectoryTaskMeta)
	var rootTaskID string
	var userInput string

	// Explicit structural metadata extracted from events.
	// taskID -> parent task id (from react_task_created / loop_marker).
	parentOf := make(map[string]string)
	// taskID / loop id -> human-readable name.
	taskName := make(map[string]string)
	// taskID -> is subagent.
	isSubAgent := make(map[string]bool)
	// node id ("taskID:loopName") -> loop metadata.
	loopMeta := make(map[string]*trajectoryTaskMeta)
	// node id -> parent node id.
	nodeParent := make(map[string]string)
	// "taskID|loopName" -> observed [first, last] event-line span of that loop
	// on that task, derived from prompt_profile events. Used to nest loops that
	// run inside another loop (e.g. fast_context inside code_audit_scan_*).
	loopSpans := make(map[string]*loopSpan)

	for _, e := range events {
		if e == nil {
			continue
		}
		line := int64(e.Model.ID)

		if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "timeline_item" {
			var item struct {
				EntryType string `json:"entry_type"`
				Content   string `json:"content"`
				Type      string `json:"type"`
			}
			if err := json.Unmarshal(e.Content, &item); err == nil {
				if item.EntryType == "current task user input" || item.Type == "user_input" {
					if userInput == "" {
						userInput = item.Content
					}
				}
			}
		}

		// react_task_created: explicit task metadata.
		if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "react_task_created" && e.Content != nil {
			var payload map[string]any
			if err := json.Unmarshal(e.Content, &payload); err == nil {
				tid := stringFrom(payload, "react_task_id")
				if tid != "" {
					if name := stringFrom(payload, "react_task_name"); name != "" {
						taskName[tid] = name
					}
					if parent := stringFrom(payload, "react_parent_task_id"); parent != "" {
						parentOf[tid] = parent
					}
					if b, ok := payload["react_task_is_sub_agent"].(bool); ok {
						isSubAgent[tid] = b
					}
				}
			}
		}

		// loop_marker: explicit phase/loop/subagent lifecycle metadata.
		if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "loop_marker" && e.Content != nil {
			var payload map[string]any
			if err := json.Unmarshal(e.Content, &payload); err == nil {
				kind := stringFrom(payload, "loop_kind")
				loopName := stringFrom(payload, "loop_name")
				tid := stringFrom(payload, "task_id")
				parent := stringFrom(payload, "parent_task_id")
				phaseName := stringFrom(payload, "phase_name")
				if loopName != "" {
					nodeID := loopNodeID(tid, loopName)
					m := loopMeta[nodeID]
					if m == nil {
						m = &trajectoryTaskMeta{taskID: nodeID, firstLine: line}
						loopMeta[nodeID] = m
					}
					m.loopName = loopName
					if phaseName != "" {
						m.phaseLabel = phaseName
					}
					if line < m.firstLine {
						m.firstLine = line
					}
					if line > m.lastLine {
						m.lastLine = line
					}
					m.blockLineNos = append(m.blockLineNos, line)
					if parent != "" {
						nodeParent[nodeID] = parent
					}
					// Remember the node kind per loop node.
					if _, ok := loopMeta[nodeID+"#kind"]; !ok {
						loopMeta[nodeID+"#kind"] = &trajectoryTaskMeta{loopName: kind}
					}
				}
				// For non-loop markers the task itself may be referenced by id.
				if tid != "" {
					if name := stringFrom(payload, "task_name"); name != "" {
						// First-write-wins: a sub-agent/phase task emits several
						// loop_markers (the subagent marker, then nested loops like
						// code_audit_scan_* and fast_context). The structural marker's
						// task_name (e.g. "Phase 2 category scan: 路径遍历...") is the
						// meaningful label; a later nested-loop marker may reuse the
						// same task_id with a generic task_name (e.g. "fast-context")
						// and must not clobber it.
						if taskName[tid] == "" {
							taskName[tid] = name
						}
					}
					if parent != "" {
						parentOf[tid] = parent
					}
					if kind == "subagent" {
						isSubAgent[tid] = true
					}
				}
			}
		}

		// focus_on / lose_focus events are emitted when the runtime dynamically
		// switches to another loop. We capture the loop name as a child of the
		// current task.
		if (e.Type == schema.EVENT_TYPE_FOCUS_ON_LOOP || e.Type == schema.EVENT_TYPE_LOSE_FOCUS_LOOP) && e.Content != nil {
			var payload struct {
				LoopName string `json:"loop_name"`
			}
			if err := json.Unmarshal(e.Content, &payload); err == nil && payload.LoopName != "" {
				tid := e.TaskId
				if tid == "" {
					tid = rootTaskID
				}
				nodeID := loopNodeID(tid, payload.LoopName)
				m := loopMeta[nodeID]
				if m == nil {
					m = &trajectoryTaskMeta{taskID: nodeID, firstLine: line}
					loopMeta[nodeID] = m
				}
				m.loopName = payload.LoopName
				if line < m.firstLine {
					m.firstLine = line
				}
				if line > m.lastLine {
					m.lastLine = line
				}
				m.blockLineNos = append(m.blockLineNos, line)
				if tid != "" {
					nodeParent[nodeID] = tid
				}
			}
		}

		tid := e.TaskId
		if tid == "" {
			continue
		}
		if rootTaskID == "" {
			rootTaskID = tid
		}
		m := meta[tid]
		if m == nil {
			m = &trajectoryTaskMeta{taskID: tid, firstLine: line}
			meta[tid] = m
		}
		if line < m.firstLine {
			m.firstLine = line
		}
		if line > m.lastLine {
			m.lastLine = line
		}
		m.blockLineNos = append(m.blockLineNos, line)

		if e.Type == schema.EVENT_TYPE_PROMPT_PROFILE {
			var obs promptObservation
			if err := json.Unmarshal(e.Content, &obs); err == nil && obs.LoopName != "" {
				// A prompt_profile carries the loop_name of the loop that produced
				// the AI reflection, but its DB task_id column may be the session
				// root (sub-loops forward events through the parent emitter). Setting
				// this on the root task's meta would let a child loop's name (e.g.
				// dir_explore during Phase 1) pollute the session root node, so the
				// frontend renders the root as "loop:dir_explore" instead of the
				// real top-level "code_security_audit". Skip the root task here; the
				// root node's loop identity is derived from the top-level loop_marker.
				if tid != rootTaskID {
					m.loopName = obs.LoopName
				}
				// Record the actual [firstLine, lastLine] span of each loop on a
				// task. loop_marker events only carry an "enter" (no "exit"), so we
				// approximate a loop's lifecycle from the prompt_profile events it
				// produced. This lets us nest loops that run *inside* another loop
				// (e.g. fast_context is entered while code_audit_scan_path_traversal
				// is still iterating) rather than rendering them as flat siblings.
				if tid != "" && obs.LoopName != "" {
					sp := loopSpans[tid+"|"+obs.LoopName]
					if sp == nil {
						sp = &loopSpan{first: line, last: line}
						loopSpans[tid+"|"+obs.LoopName] = sp
					}
					if line < sp.first {
						sp.first = line
					}
					if line > sp.last {
						sp.last = line
					}
				}
			}
		}
		if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "timeline_item" {
			var item struct {
				EntryType string `json:"entry_type"`
				Content   string `json:"content"`
			}
			if err := json.Unmarshal(e.Content, &item); err == nil {
				switch {
				case strings.HasPrefix(item.EntryType, "[PHASE"):
					m.phaseLabel = item.Content
				case strings.HasPrefix(item.EntryType, "[EXPLORE_START]"):
					m.loopLabel = item.Content
					if m.phaseLabel == "" {
						m.phaseLabel = item.Content
					}
				case strings.HasPrefix(item.EntryType, "[AUDIT_START]") ||
					strings.HasPrefix(item.EntryType, "[PLAN_START]") ||
					strings.HasPrefix(item.EntryType, "[VERIFY_START]") ||
					strings.HasPrefix(item.EntryType, "[REPORT_START]") ||
					strings.HasPrefix(item.EntryType, "[AUDIT_FOLLOWUP]"):
					m.loopLabel = item.Content
				case item.EntryType == "iteration":
					if m.summary == "" {
						m.summary = item.Content
					}
				}
			}
		}
	}

	// Ensure root exists even if no task_id events.
	if rootTaskID == "" {
		rootTaskID = sessionID
	}
	if meta[rootTaskID] == nil {
		meta[rootTaskID] = &trajectoryTaskMeta{taskID: rootTaskID, firstLine: metaFirstLine(meta)}
	}

	// classifyKind decides the visual category of a task node from its id and
	// explicit sub-agent flag. loop_marker loop_kind refines this further during
	// the merge step below.
	classifyKind := func(tid string) string {
		if tid == rootTaskID {
			return "session"
		}
		if isSubAgent[tid] {
			return "subagent"
		}
		// Legacy fallback for old fixtures without loop_marker / react_parent_task_id.
		if strings.Contains(tid, "-sub-") {
			return "subagent"
		}
		if strings.Contains(tid, "-phase") {
			return "phase"
		}
		return "subagent"
	}

	// nodeLabel picks the most informative human-readable label for a task.
	nodeLabel := func(tid, kind, loopName string, m *trajectoryTaskMeta) string {
		switch kind {
		case "session":
			if userInput != "" {
				return truncate(userInput, 80)
			}
			if name := taskName[tid]; name != "" {
				return name
			}
			return "main task"
		case "phase":
			if m.phaseLabel != "" {
				return m.phaseLabel
			}
			if m.loopLabel != "" {
				return m.loopLabel
			}
			return loopName
		case "loop":
			if m.phaseLabel != "" {
				return m.phaseLabel
			}
			if m.loopLabel != "" {
				return m.loopLabel
			}
			return loopName
		default: // subagent
			if name := taskName[tid]; name != "" {
				return name
			}
			if m.loopLabel != "" {
				return m.loopLabel
			}
			return loopName
		}
	}

	// --- Phase A: build one canonical node per task id. ---
	// loop_marker events are merged INTO the owning task's node (one node, not a
	// separate synthetic sibling), which eliminates the duplicate-loop-node /
	// duplicate-task-node problem the old rebuildTrajectoryFromLoops produced.
	nodeOf := make(map[string]*TrajectoryNode, len(meta))
	taskIDsByFirstLine := make([]string, 0, len(meta))
	for tid := range meta {
		taskIDsByFirstLine = append(taskIDsByFirstLine, tid)
	}
	sort.SliceStable(taskIDsByFirstLine, func(i, j int) bool {
		return meta[taskIDsByFirstLine[i]].firstLine < meta[taskIDsByFirstLine[j]].firstLine
	})

	for _, tid := range taskIDsByFirstLine {
		m := meta[tid]
		kind := classifyKind(tid)
		loopName := m.loopName
		// The session root must not inherit a child loop's name from a
		// prompt_profile whose DB task_id is the root (it would render the root
		// as e.g. "loop:dir_explore" instead of "code_security_audit").
		if kind == "session" {
			loopName = ""
		}
		summary := truncate(m.summary, 120)
		if (summary == "" || kind == "session") && m.loopLabel != "" {
			summary = truncate(m.loopLabel, 120)
		}
		nodeOf[tid] = &TrajectoryNode{
			NodeID:       tid,
			Kind:         kind,
			Label:        nodeLabel(tid, kind, loopName, m),
			LoopName:     loopName,
			EnterLine:    m.firstLine,
			ExitLine:     m.lastLine,
			Summary:      summary,
			BlockLineNos: append([]int64(nil), m.blockLineNos...),
		}
	}

	// --- Phase B: merge loop_marker data into the canonical task nodes. ---
	// Each loop_marker has a synthetic key "taskID:loopName". Its owning task is
	// taskID; we fold the loop's name, lifecycle and kind into that task's node
	// rather than creating a sibling. loop_kind can upgrade the node's Kind to
	// something more specific than string inference (e.g. the top-level
	// code_security_audit loop_marker turns the session-root carrier into a
	// distinct "loop" child of the session).
	// --- Phase B: merge loop_marker data into task nodes; spawn nested loops. ---
	// Model (derived from the confirmed code_security_audit event stream):
	//   - loop_kind "phase" / "subagent": a structural marker for the carrier
	//     task itself. Fold its kind/name into the task's canonical node (no new
	//     node). E.g. a "-phase2" task whose phase marker is
	//     code_audit_phase2_orchestrator becomes Kind=phase, LoopName=that.
	//   - loop_kind "loop": a ReAct loop. If it runs on the root task with no
	//     parent (the top-level code_security_audit loop), materialize it as a
	//     distinct "loop" child of the session. If it runs on a subtask, it is a
	//     NESTED loop (report_generating, code_audit_scan_*, fast_context,
	//     code_audit_verify_*): spawn a loop node nested under the carrier task.
	type loopMarkerInfo struct {
		owningTask string
		loopName   string
		loopKind   string
		parentTask string // explicit parent_task_id (empty -> carrier task)
		firstLine  int64
		lastLine   int64
		blockLines []int64
	}
	var loopMarkers []loopMarkerInfo
	for nodeID, m := range loopMeta {
		if strings.HasSuffix(nodeID, "#kind") {
			continue
		}
		parts := strings.SplitN(nodeID, ":", 2)
		owningTask := nodeID
		if len(parts) == 2 {
			owningTask = parts[0]
		}
		kind := "loop"
		if km := loopMeta[nodeID+"#kind"]; km != nil && km.loopName != "" {
			kind = km.loopName
		}
		loopMarkers = append(loopMarkers, loopMarkerInfo{
			owningTask: owningTask,
			loopName:   m.loopName,
			loopKind:   kind,
			parentTask: nodeParent[nodeID],
			firstLine:  m.firstLine,
			lastLine:   m.lastLine,
			blockLines: m.blockLineNos,
		})
	}
	// Deterministic ordering by firstLine.
	sort.SliceStable(loopMarkers, func(i, j int) bool {
		return loopMarkers[i].firstLine < loopMarkers[j].firstLine
	})

	// nestedLoops[nodeID] collects loop/phase nodes spawned under a given parent
	// node id. Phase markers emitted on the root task each become a distinct
	// phase child (Phase 1/2/3/4) rather than collapsing into the session node.
	nestedLoops := make(map[string][]*TrajectoryNode)
	// taskLoops[taskID] collects nested loop nodes spawned on a task, so we can
	// nest loops that run inside another loop on the same task.
	taskLoops := make(map[string][]*TrajectoryNode)
	// loopCarrier[nodeID] remembers the explicit parent_task_id for a spawned
	// loop node, used when the loop has no containing outer loop.
	loopCarrier := make(map[string]string)
	// Whether the root task carried a top-level orchestrator loop marker.
	var topLevelLoopName string
	for _, lm := range loopMarkers {
		switch lm.loopKind {
		case "subagent":
			// Structural marker for a forked sub-agent: fold its name into the
			// carrier task's canonical node (one node per sub-agent task).
			node := nodeOf[lm.owningTask]
			if node == nil {
				continue
			}
			node.Kind = "subagent"
			if lm.loopName != "" {
				node.LoopName = lm.loopName
			}
			if lm.firstLine != 0 && (node.EnterLine == 0 || lm.firstLine < node.EnterLine) {
				node.EnterLine = lm.firstLine
			}
			if lm.lastLine != 0 && (node.ExitLine == 0 || lm.lastLine > node.ExitLine) {
				node.ExitLine = lm.lastLine
			}
			node.BlockLineNos = mergeLineNos(node.BlockLineNos, lm.blockLines)
			continue
		case "phase":
			// A phase marker names an orchestration stage (Phase 1/2/3/4). On the
			// root task each phase is a distinct child node under the top-level
			// audit loop; on a subtask it would label that task, but in practice
			// phase markers are emitted on the root task.
			if lm.owningTask == rootTaskID {
				parentTask := lm.parentTask
				if parentTask == "" {
					parentTask = rootTaskID
				}
				phaseNode := &TrajectoryNode{
					NodeID:       loopNodeID(lm.owningTask, lm.loopName),
					Kind:         "phase",
					Label:        lm.loopName,
					LoopName:     lm.loopName,
					EnterLine:    lm.firstLine,
					ExitLine:     lm.lastLine,
					BlockLineNos: append([]int64(nil), lm.blockLines...),
				}
				nestedLoops[parentTask] = append(nestedLoops[parentTask], phaseNode)
				nodeOf[phaseNode.NodeID] = phaseNode
				continue
			}
			// Phase marker on a subtask: fold into the carrier task node.
			node := nodeOf[lm.owningTask]
			if node == nil {
				continue
			}
			node.Kind = "phase"
			if lm.loopName != "" {
				node.LoopName = lm.loopName
			}
			if lm.firstLine != 0 && (node.EnterLine == 0 || lm.firstLine < node.EnterLine) {
				node.EnterLine = lm.firstLine
			}
			if lm.lastLine != 0 && (node.ExitLine == 0 || lm.lastLine > node.ExitLine) {
				node.ExitLine = lm.lastLine
			}
			node.BlockLineNos = mergeLineNos(node.BlockLineNos, lm.blockLines)
		case "loop":
			if lm.owningTask == rootTaskID {
				// Top-level orchestrator loop on the session task: becomes the
				// single "loop" child of the session (materialized in Phase C).
				if topLevelLoopName == "" {
					topLevelLoopName = lm.loopName
				}
				continue
			}
			// Nested loop running on a subtask. Collect it per carrier task; the
			// actual nesting (a loop entered while another loop on the same task
			// is still iterating, e.g. fast_context inside code_audit_scan_*) is
			// resolved after the loopMarkers pass using prompt_profile spans.
			parentTask := lm.parentTask
			if parentTask == "" {
				parentTask = lm.owningTask
			}
			// Refine the loop's [enter, exit] range from the prompt_profile span
			// when available (loop_marker only carries "enter").
			enter, exit := lm.firstLine, lm.lastLine
			if sp := loopSpans[lm.owningTask+"|"+lm.loopName]; sp != nil {
				if sp.first != 0 && (enter == 0 || sp.first < enter) {
					enter = sp.first
				}
				if sp.last != 0 && (exit == 0 || sp.last > exit) {
					exit = sp.last
				}
			}
			loopNode := &TrajectoryNode{
				NodeID:       loopNodeID(lm.owningTask, lm.loopName),
				Kind:         "loop",
				Label:        lm.loopName,
				LoopName:     lm.loopName,
				EnterLine:    enter,
				ExitLine:     exit,
				BlockLineNos: append([]int64(nil), lm.blockLines...),
			}
			taskLoops[lm.owningTask] = append(taskLoops[lm.owningTask], loopNode)
			nodeOf[loopNode.NodeID] = loopNode
			// Remember the explicit carrier (parent_task_id) for Phase C.
			loopCarrier[loopNode.NodeID] = parentTask
		}
	}

	// Resolve nesting among loops that run on the same task. A loop whose
	// [enter, exit] span is fully contained inside another loop's span on the
	// same task is a child of that outer loop (e.g. fast_context entered while
	// code_audit_scan_path_traversal is still iterating). Otherwise the loop
	// attaches directly under its carrier task.
	for taskID, loops := range taskLoops {
		// Sort by enter line so outer loops come first.
		sort.SliceStable(loops, func(i, j int) bool { return loops[i].EnterLine < loops[j].EnterLine })
		for _, inner := range loops {
			var outer *TrajectoryNode
			for _, cand := range loops {
				if cand == inner {
					continue
				}
				// cand is the outer if it starts before inner and ends after inner.
				if cand.EnterLine <= inner.EnterLine && (cand.ExitLine == 0 || cand.ExitLine >= inner.ExitLine) {
					// Pick the tightest containing loop (largest enter among
					// containers).
					if outer == nil || cand.EnterLine > outer.EnterLine {
						outer = cand
					}
				}
			}
			if outer != nil {
				outer.Children = append(outer.Children, inner)
			} else {
				// No containing loop: attach under the carrier task.
				carrier := loopCarrier[inner.NodeID]
				if carrier == "" {
					carrier = taskID
				}
				nestedLoops[carrier] = append(nestedLoops[carrier], inner)
			}
		}
	}

	// --- Phase C: determine each node's parent and assemble the tree. ---
	// Parent resolution priority:
	//   1. loop_marker parent_task_id / nested-loop carrier (explicit)
	//   2. react_task_created react_parent_task_id (explicit)
	//   3. inferParentTaskID string fallback
	root := nodeOf[rootTaskID]
	if root == nil {
		root = &TrajectoryNode{NodeID: rootTaskID, Kind: "session", Label: "main task"}
		nodeOf[rootTaskID] = root
	}

	// Materialize the top-level orchestrator loop as the single child of the
	// session; all phases/subagents whose parent resolves to the root nest under
	// it rather than directly under the session node.
	var topLevelLoop *TrajectoryNode
	if topLevelLoopName != "" {
		rm := meta[rootTaskID]
		topLevelLoop = &TrajectoryNode{
			NodeID:    loopNodeID(rootTaskID, topLevelLoopName),
			Kind:      "loop",
			Label:     topLevelLoopName,
			LoopName:  topLevelLoopName,
			EnterLine: rm.firstLine,
			ExitLine:  rm.lastLine,
		}
		nodeOf[topLevelLoop.NodeID] = topLevelLoop
	}

	// resolveParent maps a task id to its parent task id using explicit metadata
	// then string inference. It never returns the task itself (cycle guard).
	resolveParent := func(tid string) string {
		if p, ok := parentOf[tid]; ok && p != "" && p != tid {
			return p
		}
		return inferParentTaskID(tid, rootTaskID)
	}

	// phaseNodeForTask maps a "-phaseN" subtask id to the phase-marker node it
	// belongs to, so phase subtasks (and their nested loops) nest under the
	// matching phase node instead of duplicating as top-level siblings.
	phaseNodeByNumber := make(map[int]*TrajectoryNode)
	for _, lm := range loopMarkers {
		if lm.loopKind == "phase" && lm.owningTask == rootTaskID {
			n := nodeOf[loopNodeID(lm.owningTask, lm.loopName)]
			if n != nil {
				// Order of emission == phase number (Phase 1/2/3/4 by firstLine).
				phaseNodeByNumber[len(phaseNodeByNumber)+1] = n
			}
		}
	}
	phaseNumberOf := func(tid string) int {
		// Only match a phase CARRIER task ("...-phaseN" exactly), not its
		// sub-agents ("...-phaseN-sub-..."), which must stay nested under the
		// phase orchestrator rather than collapse into the phase node.
		if strings.Contains(tid, "-sub-") {
			return 0
		}
		if idx := strings.LastIndex(tid, "-phase"); idx > 0 {
			numStr := strings.TrimSpace(tid[idx+len("-phase"):])
			digits := ""
			for _, r := range numStr {
				if r < '0' || r > '9' {
					break
				}
				digits += string(r)
			}
			if digits != "" {
				if n, err := strconv.Atoi(digits); err == nil {
					return n
				}
			}
		}
		return 0
	}

	// Attach every non-root TASK node under its resolved parent.
	for _, tid := range taskIDsByFirstLine {
		if tid == rootTaskID {
			continue
		}
		node := nodeOf[tid]
		if node == nil {
			continue
		}
		// Skip nodes that are purely loop-carrier synthetic ids (have a ":").
		if strings.Contains(tid, ":") {
			continue
		}
		// A "-phaseN" subtask belongs to its phase-marker node (so phase content
		// nests under Phase 1/2/3/4 rather than duplicating at top level).
		if pn := phaseNumberOf(tid); pn > 0 {
			if phNode, ok := phaseNodeByNumber[pn]; ok {
				// Fold the subtask's lifecycle into the phase node, then skip the
				// standalone subtask node (it is the phase node's carrier).
				if node.EnterLine != 0 && (phNode.EnterLine == 0 || node.EnterLine < phNode.EnterLine) {
					phNode.EnterLine = node.EnterLine
				}
				if node.ExitLine != 0 && (phNode.ExitLine == 0 || node.ExitLine > phNode.ExitLine) {
					phNode.ExitLine = node.ExitLine
				}
				phNode.BlockLineNos = mergeLineNos(phNode.BlockLineNos, node.BlockLineNos)
				if node.Summary != "" && phNode.Summary == "" {
					phNode.Summary = node.Summary
				}
				delete(nodeOf, tid)
				continue
			}
		}
		parentID := resolveParent(tid)
		var parent *TrajectoryNode
		switch {
		case parentID == rootTaskID && topLevelLoop != nil:
			// A phase/subagent whose parent is the root nests under the top-level
			// audit loop (its real container), not directly under the session.
			parent = topLevelLoop
		case parentID == rootTaskID:
			parent = root
		default:
			parent = nodeOf[parentID]
			if parent == nil {
				if topLevelLoop != nil {
					parent = topLevelLoop
				} else {
					parent = root
				}
			}
		}
		parent.Children = append(parent.Children, node)
	}

	// Attach spawned nested loop nodes under their carrier task.
	for parentTask, loops := range nestedLoops {
		parent := nodeOf[parentTask]
		if parent == nil || parentTask == rootTaskID {
			// Carrier unknown or the session root: phase/loop markers emitted on
			// the root task belong under the top-level audit loop (their real
			// container), not directly under the session node.
			if topLevelLoop != nil {
				parent = topLevelLoop
			} else {
				parent = root
			}
		}
		for _, lp := range loops {
			parent.Children = append(parent.Children, lp)
		}
	}

	if topLevelLoop != nil {
		root.Children = append(root.Children, topLevelLoop)
	}

	// --- Phase D: recursive sort by enter line + cycle hardening. ---
	var sortRecursively func(n *TrajectoryNode, seen map[string]bool)
	sortRecursively = func(n *TrajectoryNode, seen map[string]bool) {
		if n == nil {
			return
		}
		if seen[n.NodeID] {
			// Defensive: a cycle would infinite-loop the frontend; drop children.
			n.Children = nil
			return
		}
		seen[n.NodeID] = true
		sort.SliceStable(n.Children, func(i, j int) bool {
			return n.Children[i].EnterLine < n.Children[j].EnterLine
		})
		for _, c := range n.Children {
			sortRecursively(c, seen)
		}
		delete(seen, n.NodeID)
	}
	sortRecursively(root, make(map[string]bool))

	return root
}

// loopNodeID builds a stable synthetic node id for a loop running on a task.
func loopNodeID(taskID, loopName string) string {
	if taskID == "" {
		return loopName
	}
	return taskID + ":" + loopName
}

// stringFrom extracts a string value from a JSON-decoded map[string]any.
func stringFrom(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// inferParentTaskID strips known sub-task suffixes to find the parent task id.
// It must prefer the immediate phase parent for nested sub-agents so that
// phase2-sub-sql_injection is nested under phase2, not directly under root.
func inferParentTaskID(tid, root string) string {
	// sub-agent suffix: ...-sub-<id>-<rand>
	if idx := strings.LastIndex(tid, "-sub-"); idx > 0 {
		candidate := tid[:idx]
		if candidate != "" {
			return candidate
		}
	}
	// phase suffix: ...-phase1, ...-phase2
	if idx := strings.LastIndex(tid, "-phase"); idx > 0 {
		candidate := tid[:idx]
		if candidate != "" {
			return candidate
		}
	}
	return root
}

func metaFirstLine(meta map[string]*trajectoryTaskMeta) int64 {
	var best int64
	for _, m := range meta {
		if best == 0 || m.firstLine < best {
			best = m.firstLine
		}
	}
	return best
}

// mergeLineNos merges two sorted-ish int64 slices and deduplicates.
func mergeLineNos(a, b []int64) []int64 {
	seen := make(map[int64]bool)
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		seen[v] = true
	}
	out := make([]int64, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var _ = regexp.MustCompile
var _ = math.Abs
var _ = strconv.Atoi
