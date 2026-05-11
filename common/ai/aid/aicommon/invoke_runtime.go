package aicommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type VerifyNextMovement struct {
	Op      string `json:"op"`
	Content string `json:"content,omitempty"`
	ID      string `json:"id"`
}

type EvidenceOperation struct {
	ID      string `json:"id"`
	Op      string `json:"op"`
	Content string `json:"content,omitempty"`
}

// VerifySatisfactionResult represents the result of user satisfaction verification
type VerifySatisfactionResult struct {
	Satisfied          bool                 `json:"satisfied"`            // Whether the user is satisfied
	Reasoning          string               `json:"reasoning"`            // The reasoning for the satisfaction status
	CompletedTaskIndex string               `json:"completed_task_index"` // Index of completed task(s), e.g., "1-1" or "1-1,1-2"
	NextMovements      []VerifyNextMovement `json:"next_movements"`       // AI's next action plan for in-progress status tracking
	Evidence           string               `json:"evidence"`             // Legacy: markdown evidence string
	EvidenceOps        []EvidenceOperation  `json:"evidence_ops"`         // Structured evidence incremental operations
	OutputFiles        []string             `json:"output_files"`         // File paths created/modified by tool execution, extracted by verify AI
}

// NewVerifySatisfactionResult creates a new VerifySatisfactionResult
func NewVerifySatisfactionResult(satisfied bool, reasoning string, completedTaskIndex string) *VerifySatisfactionResult {
	return &VerifySatisfactionResult{
		Satisfied:          satisfied,
		Reasoning:          reasoning,
		CompletedTaskIndex: completedTaskIndex,
	}
}

// NewVerifySatisfactionResultWithNextMovements creates a new VerifySatisfactionResult with next movements
func NewVerifySatisfactionResultWithNextMovements(satisfied bool, reasoning string, completedTaskIndex string, nextMovements []VerifyNextMovement) *VerifySatisfactionResult {
	return &VerifySatisfactionResult{
		Satisfied:          satisfied,
		Reasoning:          reasoning,
		CompletedTaskIndex: completedTaskIndex,
		NextMovements:      nextMovements,
	}
}

func FormatVerifyNextMovementsSummary(nextMovements []VerifyNextMovement) string {
	if len(nextMovements) == 0 {
		return ""
	}
	parts := make([]string, 0, len(nextMovements))
	for _, movement := range nextMovements {
		switch strings.ToLower(strings.TrimSpace(movement.Op)) {
		case "add":
			if movement.Content == "" {
				parts = append(parts, "ADD["+movement.ID+"]")
				continue
			}
			parts = append(parts, "ADD["+movement.ID+"]: "+movement.Content)
		case "doing":
			parts = append(parts, "DOING["+movement.ID+"]")
		case "done":
			parts = append(parts, "DONE["+movement.ID+"]")
		case "delete":
			parts = append(parts, "DELETE["+movement.ID+"]")
		default:
			parts = append(parts, strings.ToUpper(movement.Op)+"["+movement.ID+"]")
		}
	}
	return strings.Join(parts, "; ")
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
	Nonce             string
	UserQuery         string
	TaskInstruction   string
	OutputExample     string
	Schema            string
	SkillsContext     string
	ExtraCapabilities string
	SessionEvidence   string
	ReactiveData      string
	InjectedMemory    string

	// RecentToolsCache 是 CACHE_TOOL_CALL 块的渲染输出 (含 directly_call_tool
	// routing hint + 最近工具的 schema/footer), 用稳定 nonce 渲染, 字节级跨 turn
	// 稳定. 物理位置在 semi-dynamic 段, 让其与 Skills + Schema 一起被
	// AI_CACHE_SEMI 边界包裹进入 prefix cache. 空字符串时模板自动跳过.
	//
	// 关键词: LoopPromptAssemblyInput, RecentToolsCache, semi-dynamic 段,
	//        AI_CACHE_SEMI prefix cache
	RecentToolsCache string

	// FrozenUserContext 用于承载 PE-TASK 等场景下"PLAN 阶段产出 + 用户原始
	// 输入"两类只读上下文。注: 命名虽为 "Frozen", 但实际并不放入冻结段;
	// 跨同一 plan 周期内同一子任务执行的多次 turn 字节稳定, 但子任务切换 +
	// EvidenceOps 嵌入 root user input 仍会让其内容抖动, 故不适合 cache。
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
	//   - v3: 迁到 semi-dynamic, 但 EvidenceOps 嵌入 root user input + 子任务
	//     切换仍让其内容抖动, 破坏 AI_CACHE_SEMI 命中;
	//   - v4 (当前): 迁到 timeline-open 末尾, 主动让其落在所有 cache 边界外,
	//     不再追求自身缓存, 而是保护更上游 SYSTEM / FROZEN / SEMI 三段缓存。
	//
	// 老路径 (普通 ReAct loop / focus mode 等没有 PLAN 上下文的场景): 此字段
	// 为空, timeline-open 段 PlanContext 子块自然不渲染, 段位置稳定。
	//
	// 关键词: FrozenUserContext, PLAN_CONTEXT 段, timeline-open 末尾注入,
	//        缓存边界外, 上游缓存保护, PE-TASK PLAN 产物
	FrozenUserContext string
}

type LoopPromptAssemblyResult struct {
	Prompt   string
	Sections any
}

type AIInvokeRuntime interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)
	AssembleLoopPrompt(tools []*aitool.Tool, input *LoopPromptAssemblyInput) (*LoopPromptAssemblyResult, error)
	SetCurrentTask(task AIStatefulTask)
	GetCurrentTask() AIStatefulTask
	GetCurrentTaskId() string

	ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error)
	ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error)
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
	AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error))
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
