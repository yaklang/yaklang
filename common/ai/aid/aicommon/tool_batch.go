package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Parameter groups contain explicit, independent tool calls in one directly_call_tool action.
// Existing size/concurrency option names and persisted keys are retained; no mode
// or parameter-generation concurrency remains. Outcomes settle before the next decision.
const (
	ConfigKeyToolBatchMaxCalls          = "tool_batch_max_calls"
	ConfigKeyToolBatchInvokeConcurrency = "tool_batch_invoke_concurrency"

	DefaultToolBatchMaxCalls          = 8
	DefaultToolBatchInvokeConcurrency = 3
)

func clampToolBatchMaxCalls(value int) int {
	if value < 2 {
		return 2
	}
	if value > DefaultToolBatchMaxCalls {
		return DefaultToolBatchMaxCalls
	}
	return value
}

func clampToolBatchConcurrency(value int) int {
	if value < 1 {
		return 1
	}
	if value > DefaultToolBatchMaxCalls {
		return DefaultToolBatchMaxCalls
	}
	return value
}

// WithToolBatchMaxCalls configures the native action-array limit. It is
// clamped to the
// action schema's minItems/maxItems contract.
func WithToolBatchMaxCalls(value int) ConfigOption {
	return func(config *Config) error {
		config.SetConfig(ConfigKeyToolBatchMaxCalls, clampToolBatchMaxCalls(value))
		return nil
	}
}

func WithToolBatchInvokeConcurrency(value int) ConfigOption {
	return func(config *Config) error {
		config.SetConfig(ConfigKeyToolBatchInvokeConcurrency, clampToolBatchConcurrency(value))
		return nil
	}
}

// ToolCallGroupRequest is the canonical representation used by both
// directly_call_tool parameter groups. Index is the model-provided array order
// and must remain stable even when calls finish in a different order.
type ToolCallGroupRequest struct {
	BatchID string              `json:"batch_id,omitempty"`
	Calls   []ToolCallGroupCall `json:"calls"`
}

type ToolCallGroupCall struct {
	Index        int                 `json:"index"`
	ToolName     string              `json:"tool_name"`
	Params       aitool.InvokeParams `json:"params"`
	Identifier   string              `json:"identifier,omitempty"`
	Expectations string              `json:"expectations,omitempty"`
	Reason       string              `json:"reason,omitempty"`

	// ExecutionCallID is assigned by the runtime, never by the model. Keeping it
	// on the request lets events, checkpoints and results identify one child.
	ExecutionCallID string `json:"execution_call_id,omitempty"`
}

type ToolCallStage string

const (
	ToolCallStageQueued           ToolCallStage = "queued"
	ToolCallStagePreparing        ToolCallStage = "preparing"
	ToolCallStageReviewing        ToolCallStage = "reviewing"
	ToolCallStageRunning          ToolCallStage = "running"
	ToolCallStagePrepareFailed    ToolCallStage = "prepare_failed"
	ToolCallStageValidationFailed ToolCallStage = "validation_failed"
	ToolCallStageInvokeFailed     ToolCallStage = "invoke_failed"
	ToolCallStageCancelled        ToolCallStage = "cancelled"
	// ToolCallStageDone is a lifecycle state: the callback settled and returned
	// a protocol-complete ToolResult. Inspect ExecutionStatus/Result for actual
	// execution semantics and the task verifier for goal satisfaction.
	ToolCallStageDone ToolCallStage = "done"
)

type ToolCallOutcome struct {
	Index           int                        `json:"index"`
	CallID          string                     `json:"call_id,omitempty"`
	RequestedTool   string                     `json:"requested_tool"`
	FinalTool       string                     `json:"final_tool,omitempty"`
	Stage           ToolCallStage              `json:"stage"`
	ExecutionStatus aitool.ToolExecutionStatus `json:"execution_status,omitempty"`
	Result          *aitool.ToolResult         `json:"result,omitempty"`
	Err             error                      `json:"-"`
	DirectlyAnswer  bool                       `json:"directly_answer,omitempty"`
}

type ToolCallGroupResult struct {
	BatchID        string            `json:"batch_id,omitempty"`
	Outcomes       []ToolCallOutcome `json:"outcomes"`
	DirectlyAnswer bool              `json:"directly_answer,omitempty"`
}

// ToolCallGroupInvokeRuntime is an optional extension rather than a method on the
// large AIInvokeRuntime interface. Existing embedders and test doubles keep
// compiling, while the production ReAct runtime can execute explicit parameter groups.
type ToolCallGroupInvokeRuntime interface {
	ExecuteToolCallGroup(
		ctx context.Context,
		task AIStatefulTask,
		request *ToolCallGroupRequest,
	) (*ToolCallGroupResult, error)
}
