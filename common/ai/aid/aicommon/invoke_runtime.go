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

type AIInvokeRuntime interface {
	GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error)
	SetCurrentTask(task AIStatefulTask)
	GetCurrentTask() AIStatefulTask
	GetCurrentTaskId() string

	ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error)
	ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error)
	AskForClarification(ctx context.Context, question string, payloads []string) string
	DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error)
	CompressLongTextWithDestination(ctx context.Context, i any, destination string, targetByteSize int64) (string, error)
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
