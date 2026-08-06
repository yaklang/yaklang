package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type TodoOutcome string

const (
	TodoOutcomeResolved  TodoOutcome = "resolved"
	TodoOutcomeDismissed TodoOutcome = "dismissed"
	TodoOutcomeDeferred  TodoOutcome = "deferred"
)

type TodoAdd struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text"`
}

type TodoUpdate struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type TodoClose struct {
	ID      string      `json:"id"`
	Outcome TodoOutcome `json:"outcome"`
	Reason  string      `json:"reason"`
	Refs    []string    `json:"refs,omitempty"`
}

// TodoDelta is the optional TODO increment carried by a normal ReAct action.
// CurrentSet distinguishes an omitted current field from an explicit null or
// empty value, which clears the current focus.
type TodoDelta struct {
	Current    *string      `json:"current,omitempty"`
	CurrentSet bool         `json:"-"`
	Add        []TodoAdd    `json:"add,omitempty"`
	Update     []TodoUpdate `json:"update,omitempty"`
	Close      []TodoClose  `json:"close,omitempty"`
}

// UnmarshalJSON preserves whether current was omitted, explicitly null, or a
// string. The default decoder cannot populate CurrentSet, which would silently
// turn an explicit focus clear into "leave focus unchanged" in stream/event
// consumers that decode TodoDelta directly.
func (d *TodoDelta) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("cannot unmarshal todo_delta into nil receiver")
	}
	var wire struct {
		Add    []TodoAdd    `json:"add"`
		Update []TodoUpdate `json:"update"`
		Close  []TodoClose  `json:"close"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*d = TodoDelta{Add: wire.Add, Update: wire.Update, Close: wire.Close}
	currentRaw, exists := fields["current"]
	if !exists {
		return nil
	}
	d.CurrentSet = true
	if strings.TrimSpace(string(currentRaw)) == "null" {
		return nil
	}
	var current string
	if err := json.Unmarshal(currentRaw, &current); err != nil {
		return fmt.Errorf("todo_delta.current must be a string or null: %w", err)
	}
	current = strings.TrimSpace(current)
	d.Current = &current
	return nil
}

// MarshalJSON preserves the current field's three-state protocol in emitted
// applied_delta payloads: omitted means unchanged, null/empty means clear.
func (d TodoDelta) MarshalJSON() ([]byte, error) {
	payload := make(map[string]any)
	if d.CurrentSet {
		if d.Current == nil {
			payload["current"] = nil
		} else {
			payload["current"] = *d.Current
		}
	}
	if len(d.Add) > 0 {
		payload["add"] = d.Add
	}
	if len(d.Update) > 0 {
		payload["update"] = d.Update
	}
	if len(d.Close) > 0 {
		payload["close"] = d.Close
	}
	return json.Marshal(payload)
}

// TodoOperation is the legacy-shaped applied_ops event projection consumed by
// existing Yakit versions. Model output never accepts this shape.
type TodoOperation struct {
	Op      string   `json:"op"`
	Content string   `json:"content,omitempty"`
	ID      string   `json:"id,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Refs    []string `json:"refs,omitempty"`
}

type EvidenceOperation struct {
	ID      string `json:"id"`
	Op      string `json:"op"`
	Content string `json:"content,omitempty"`
}

// VerifySatisfactionResult represents the result of user satisfaction verification
type VerifySatisfactionResult struct {
	Satisfied          bool                `json:"satisfied"`
	Reasoning          string              `json:"reasoning"`
	CompletedTaskIndex string              `json:"completed_task_index"`
	Evidence           string              `json:"evidence"`
	EvidenceOps        []EvidenceOperation `json:"evidence_ops"`
}

// NewVerifySatisfactionResult creates a new VerifySatisfactionResult
func NewVerifySatisfactionResult(satisfied bool, reasoning string, completedTaskIndex string) *VerifySatisfactionResult {
	return &VerifySatisfactionResult{
		Satisfied:          satisfied,
		Reasoning:          reasoning,
		CompletedTaskIndex: completedTaskIndex,
	}
}

func FormatEvidenceOpLine(op EvidenceOperation, language string) string {
	id := strings.TrimSpace(op.ID)
	content := strings.TrimSpace(op.Content)
	firstLine := ""
	if content != "" {
		firstLine = strings.SplitN(content, "\n", 2)[0]
	}

	isCN := strings.Contains(strings.ToLower(language), "zh") ||
		strings.Contains(strings.ToLower(language), "chinese")

	switch strings.ToLower(strings.TrimSpace(op.Op)) {
	case "add":
		if id == "" && content == "" {
			return ""
		}
		if isCN {
			if id != "" && firstLine != "" {
				return fmt.Sprintf("- **新发现**: %s `#%s`", firstLine, id)
			}
			if firstLine != "" {
				return fmt.Sprintf("- **新发现**: %s", firstLine)
			}
			return fmt.Sprintf("- **新发现**: `#%s`", id)
		}
		if id != "" && firstLine != "" {
			return fmt.Sprintf("- **New finding**: %s `#%s`", firstLine, id)
		}
		if firstLine != "" {
			return fmt.Sprintf("- **New finding**: %s", firstLine)
		}
		return fmt.Sprintf("- **New finding**: `#%s`", id)
	case "update":
		if id == "" {
			return ""
		}
		if isCN {
			if firstLine != "" {
				return fmt.Sprintf("- **更新证据**: %s `#%s`", firstLine, id)
			}
			return fmt.Sprintf("- **更新证据**: `#%s`", id)
		}
		if firstLine != "" {
			return fmt.Sprintf("- **Updated**: %s `#%s`", firstLine, id)
		}
		return fmt.Sprintf("- **Updated**: `#%s`", id)
	case "delete":
		if id == "" {
			return ""
		}
		if isCN {
			return fmt.Sprintf("- **移除过时信息**: `#%s`", id)
		}
		return fmt.Sprintf("- **Removed outdated**: `#%s`", id)
	default:
		if id == "" && content == "" {
			return ""
		}
		label := strings.ToUpper(strings.TrimSpace(op.Op))
		if label == "" {
			label = "?"
		}
		if id != "" && firstLine != "" {
			return fmt.Sprintf("- **%s**: %s `#%s`", label, firstLine, id)
		}
		if firstLine != "" {
			return fmt.Sprintf("- **%s**: %s", label, firstLine)
		}
		return fmt.Sprintf("- **%s**: `#%s`", label, id)
	}
}

func FormatEvidenceOpsLines(ops []EvidenceOperation, language string) string {
	var lines []string
	for _, op := range ops {
		line := FormatEvidenceOpLine(op, language)
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// SelectedKnowledgeBaseResult represents the result of knowledge base selection
type SelectedKnowledgeBaseResult struct {
	Reason         string   `json:"reason"`          // The reasoning for the selection
	KnowledgeBases []string `json:"knowledge_bases"` // The selected knowledge base names/IDs
}

// NewSelectedKnowledgeBaseResult creates a new SelectedKnowledgeBaseResult
func NewSelectedKnowledgeBaseResult(reason string, knowledgeBases []string) *SelectedKnowledgeBaseResult {
	return &SelectedKnowledgeBaseResult{
		Reason:         reason,
		KnowledgeBases: knowledgeBases,
	}
}

type LoopPromptAssemblyInput struct {
	Nonce string

	// Lightweight selects the bounded speed-priority projection: the full
	// frozen session history is replaced by a recent Timeline window and large
	// auxiliary context fields are capped. The action schema remains intact so
	// execution semantics do not change.
	Lightweight bool

	UserQuery         string
	TaskInstruction   string
	OutputExample     string
	Schema            string
	SkillsContext     string
	ExtraCapabilities string
	SessionEvidence   string
	// TodoSnapshot 是全局 TODO 列表的渲染输出 (含 <|TODO_LIST_<nonce>|>...
	// 边界标签的整段块), 紧跟在 SessionEvidence 后面注入到 timeline-open 段。
	// 与 SessionEvidence 一样落在所有缓存边界外, 保证不污染上游 prefix cache.
	// 空字符串时 timeline-open 模板自动跳过该块。
	//
	// 关键词: TodoSnapshot, 全局 TODO 块, timeline-open 段, SessionEvidence 后,
	//        loop prompt 任何时刻可见
	TodoSnapshot   string
	ReactiveData   string
	InjectedMemory string
	// TodoCheckpoint is the one-shot soft-STW prompt tail. It is rendered as
	// the final child of the pure dynamic section, immediately before the
	// PROMPT_SECTION_dynamic_END marker, and never enters a cacheable section.
	TodoCheckpoint string

	// FrozenUserContext 用于承载 PE-TASK 等场景下"PLAN 阶段产出 + 用户原始
	// 输入"两类只读上下文。注: 命名虽为 "Frozen", 但实际并不放入冻结段;
	// 跨同一 plan 周期内同一子任务执行的多次 turn 字节稳定, 但子任务切换仍
	// 会让其内容抖动, 故不适合 cache。
	//
	// 物理位置: 包装为 <|PLAN_CONTEXT_<stable-nonce>|>...<|PLAN_CONTEXT_END_
	// <stable-nonce>|> 后, 注入到 timeline-open 段最末尾 (UserHistory 之后)。
	// timeline-open 段不被 AI_CACHE_FROZEN / AI_CACHE_SEMI 任何缓存边界包裹,
	// 是"易变尾段"。
	//
	// 设计取舍 (历史演进):
	//   - v1: 注入 dynamic 段 (turn nonce), 完全不可缓存;
	//   - v2: 迁到 frozen-block, 但 root task / 普通 ReAct 时为空, 渲染态
	//     抖动破坏 AI_CACHE_FROZEN 命中;
	//   - v3: 迁到 semi-dynamic, 但子任务切换仍让其内容抖动, 破坏
	//     AI_CACHE_SEMI 命中;
	//   - v4 (当前): 迁到 timeline-open 末尾, 主动让其落在所有 cache 边界外,
	//     不再追求自身缓存, 而是保护更上游 SYSTEM / FROZEN / SEMI 三段缓存。
	//
	// 老路径 (普通 ReAct loop / focus mode 等没有 PLAN 上下文的场景): 此字段
	// 为空, timeline-open 段 PlanContext 子块自然不渲染, 段位置稳定。
	//
	// 关键词: FrozenUserContext, PLAN_CONTEXT 段, timeline-open 末尾注入,
	//        缓存边界外, 上游缓存保护, PE-TASK PLAN 产物
	FrozenUserContext string

	// FrozenPartitions 是业务侧提供的通用 frozen-block 分区。共享模板只识别
	// FrozenBlockPartition，不直接依赖 planAndExec / FACTS / DOCUMENT 等业务类型。
	FrozenPartitions []FrozenBlockPartition

	// ForcedSkills 是「用户强制加载」SKILL 的满内容渲染 (含 USER_FORCED_SKILL 边界).
	// 物理位置: frozen_block 顶部 (比 SKILLS_CONTEXT 目录层级还高), 最高优先级.
	// 用户主动加载是偶发行为, 加载后整段字节稳定, 利于 frozen_block 缓存; 热加载时
	// 接受重缓存. 空字符串时 frozen_block 顶部子块自动跳过.
	//
	// 关键词: ForcedSkills, 用户强制加载, frozen_block 顶部, 最高优先级
	ForcedSkills string

	// AutoLoadedSkills 是「AI 意图驱动加载」SKILL 的渲染 (含 AUTO_LOADED_SKILLS 边界).
	// 物理位置: semi_dynamic_2 尾部 (TaskInstruction/Schema/OutputExample 之后).
	// 关键缓存点: semi2 前缀字节不变 → AI_CACHE_SEMI2 前缀缓存命中, 仅尾部变化.
	// 空字符串时 semi_dynamic_2 尾部子块自动跳过.
	//
	// 关键词: AutoLoadedSkills, AI 意图驱动, semi_dynamic_2 尾部, LRU 折叠
	AutoLoadedSkills string
}

type LoopPromptAssemblyResult struct {
	Prompt   string
	Sections any
}

// ExecutePlanInput carries an approved plan generated outside Coordinator plan loop.
type ExecutePlanInput struct {
	PlanPayload  string
	PlanData     string
	PlanFacts    string
	PlanDocument string
}

// PlanCoordinatorSession keeps a plan-exec coordinator alive across review and async execution.
type PlanCoordinatorSession interface {
	CoordinatorID() string
	ReviewPlan(ctx context.Context) error
	ApprovedPlanInput() *ExecutePlanInput
	Close()
}

type AIInvokeRuntime interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)
	AssembleLoopPrompt(tools []*aitool.Tool, input *LoopPromptAssemblyInput) (*LoopPromptAssemblyResult, error)
	SetCurrentTask(task AIStatefulTask)
	GetCurrentTask() AIStatefulTask
	GetCurrentTaskId() string
	// AddRuntimeTask appends a task to the runtime task list in a thread-safe manner.
	AddRuntimeTask(task AIStatefulTask)

	ExecuteToolRequiredAndCall(ctx context.Context, name string, opt ...ToolCallerOption) (*aitool.ToolResult, bool, error)
	ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams, opt ...ToolCallerOption) (*aitool.ToolResult, bool, error)
	// DirectlyCallTool handles a directly_call_tool action: it creates the tool-call
	// card (loading) first, then reads reason/params from the streaming action and
	// invokes the tool. The loop-layer prepare callback does param normalize/validate
	// and may signal fallbackToRequire to reuse the same card and switch to the AI
	// param-generation path. See aicommon.ToolCaller.DirectlyCallTool / DirectlyCallPrepareFunc.
	DirectlyCallTool(ctx context.Context, toolName string, action *Action, prepare DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error)
	AskForClarification(ctx context.Context, question string, payloads []string) string
	DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error)
	CompressLongTextWithDestination(ctx context.Context, i any, destination string, targetByteSize int64) (string, error)
	// QuickKnowledgeSearch performs a fast local knowledge-base search using LIKE + BM25.
	QuickKnowledgeSearch(ctx context.Context, query string, keywords []string, collections ...string) (string, error)
	// EnhanceKnowledgeGetterEx 支持多种 EnhancePlan 的知识增强获取器
	// enhancePlans 参数可选，支持：
	//   - nil 或空切片：使用默认完整增强流程
	//   - []string{"exact_keyword_search"}: 仅使用精准关键词搜索
	//   - []string{"hypothetical_answer", "generalize_query"}: 指定增强策略组合
	EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error)
	// VerifyUserSatisfaction verifies if the user is satisfied with the result
	VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*VerifySatisfactionResult, error)
	RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error))
	AsyncPlanOnly(ctx context.Context, planPayload string, onFinish func(error))
	AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error))
	ReviewExecutePlan(ctx context.Context, input *ExecutePlanInput) (*ExecutePlanInput, error)
	ForceReviewExecutePlan(ctx context.Context, input *ExecutePlanInput) (*ExecutePlanInput, error)
	BeginPlanCoordinatorSession(ctx context.Context, input *ExecutePlanInput, forceManualReview bool) (PlanCoordinatorSession, error)
	PublishDetachedPlan(ctx context.Context, input *ExecutePlanInput, reactTaskID string) (coordinatorID string, err error)
	AsyncExecutePlan(ctx context.Context, input *ExecutePlanInput, onFinish func(error))
	AsyncExecuteCod(ctx context.Context, coordinatorID string, onFinish func(error))
	InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error)
	InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error)
	InvokeQualityPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...GeneralKVConfigOption) (*Action, error)
	// SelectKnowledgeBase selects appropriate knowledge bases based on the user query
	// It uses AI to analyze the query and match it with available knowledge bases
	SelectKnowledgeBase(ctx context.Context, originQuery string) (*SelectedKnowledgeBaseResult, error)

	ExecuteLoopTaskIF(taskTypeName string, task AIStatefulTask, options ...any) (bool, error)
	// timeline operator
	AddToTimeline(entry, content string)

	GetConfig() AICallerConfigIf
	EmitFileArtifactWithExt(name, ext string, data any) string
	EmitResultAfterStream(any)
	EmitResult(any)
}

type AITaskInvokeRuntime interface {
	AIInvokeRuntime
	SetCurrentTask(task AIStatefulTask)
	GetCurrentTask() AIStatefulTask
}

var AIRuntimeInvokerGetter = func(ctx context.Context, options ...ConfigOption) (AITaskInvokeRuntime, error) {
	return nil, utils.Errorf("not registered default AI runtime invoker")
}

func RegisterDefaultAIRuntimeInvoker(getter func(ctx context.Context, options ...ConfigOption) (AITaskInvokeRuntime, error)) {
	AIRuntimeInvokerGetter = getter
}
