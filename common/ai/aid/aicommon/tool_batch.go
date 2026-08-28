package aicommon

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Tool batch configuration is deliberately independent from tool_compose.
// A tool batch is one ReAct action containing mutually independent calls; it
// has no dependency graph and all of its outcomes are joined before the next
// ReAct iteration starts.
const (
	ConfigKeyToolBatchMaxCalls          = "tool_batch_max_calls"
	ConfigKeyToolBatchParamConcurrency  = "tool_batch_param_concurrency"
	ConfigKeyToolBatchInvokeConcurrency = "tool_batch_invoke_concurrency"

	DefaultToolBatchMaxCalls          = 8
	DefaultToolBatchParamConcurrency  = 2
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
// intentionally independent from ToolComposeConcurrency and clamped to the
// action schema's minItems/maxItems contract.
func WithToolBatchMaxCalls(value int) ConfigOption {
	return func(config *Config) error {
		config.SetConfig(ConfigKeyToolBatchMaxCalls, clampToolBatchMaxCalls(value))
		return nil
	}
}

func WithToolBatchParamConcurrency(value int) ConfigOption {
	return func(config *Config) error {
		config.SetConfig(ConfigKeyToolBatchParamConcurrency, clampToolBatchConcurrency(value))
		return nil
	}
}

func WithToolBatchInvokeConcurrency(value int) ConfigOption {
	return func(config *Config) error {
		config.SetConfig(ConfigKeyToolBatchInvokeConcurrency, clampToolBatchConcurrency(value))
		return nil
	}
}

type ToolCallMode string

const (
	ToolCallModeDirect  ToolCallMode = "direct"
	ToolCallModeRequire ToolCallMode = "require"
)

// ToolBatchRequest is the canonical representation used by both
// directly_call_tool and require_tool. Index is the model-provided array order
// and must remain stable even when calls finish in a different order.
type ToolBatchRequest struct {
	BatchID string          `json:"batch_id,omitempty"`
	Calls   []ToolBatchCall `json:"calls"`
}

type ToolBatchCall struct {
	Index        int                 `json:"index"`
	Mode         ToolCallMode        `json:"mode"`
	ToolName     string              `json:"tool_name"`
	Params       aitool.InvokeParams `json:"params,omitempty"`
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

type ToolBatchResult struct {
	BatchID        string            `json:"batch_id,omitempty"`
	Outcomes       []ToolCallOutcome `json:"outcomes"`
	DirectlyAnswer bool              `json:"directly_answer,omitempty"`
}

// ToolBatchInvokeRuntime is an optional extension rather than a method on the
// large AIInvokeRuntime interface. Existing embedders and test doubles keep
// compiling, while the production ReAct runtime can opt into native batches.
type ToolBatchInvokeRuntime interface {
	ExecuteToolBatch(
		ctx context.Context,
		task AIStatefulTask,
		request *ToolBatchRequest,
	) (*ToolBatchResult, error)
}
