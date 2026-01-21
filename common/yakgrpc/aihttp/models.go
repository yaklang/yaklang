package aihttp

import (
	"time"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// RunStatus represents the status of a run
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusCancelled RunStatus = "cancelled"
	RunStatusFailed    RunStatus = "failed"
)

// CreateRunRequest is the request body for POST /agent/run
type CreateRunRequest struct {
	// TaskID is the client-provided task identifier
	TaskID string `json:"task_id"`
	// Query is the user's query/prompt
	Query string `json:"query"`
	// Params contains optional AI configuration parameters
	Params *AIParams `json:"params,omitempty"`
	// AttachedFiles contains paths to files to attach
	AttachedFiles []string `json:"attached_files,omitempty"`
}

// AIParams contains AI configuration parameters
type AIParams struct {
	// ForgeName is the AI template/forge to use
	ForgeName string `json:"forge_name,omitempty"`
	// ForgeParams are parameters for the forge
	ForgeParams map[string]string `json:"forge_params,omitempty"`
	// ReviewPolicy: "manual", "yolo", or "ai"
	ReviewPolicy string `json:"review_policy,omitempty"`
	// AIService is the AI service to use
	AIService string `json:"ai_service,omitempty"`
	// DisableToolUse disables tool usage
	DisableToolUse bool `json:"disable_tool_use,omitempty"`
	// EnableAISearchTool enables AI tool search
	EnableAISearchTool bool `json:"enable_ai_search_tool,omitempty"`
	// ReActMaxIteration sets max iteration count
	ReActMaxIteration int64 `json:"react_max_iteration,omitempty"`
	// TimelineSessionID for multi-turn conversation
	TimelineSessionID string `json:"timeline_session_id,omitempty"`
}

// CreateRunResponse is the response for POST /agent/run
type CreateRunResponse struct {
	// RunID is the server-assigned run identifier
	RunID string `json:"run_id"`
	// TaskID is the client-provided task identifier
	TaskID string `json:"task_id"`
	// StartTime is when the run started
	StartTime time.Time `json:"start_time"`
	// Status is the current run status
	Status RunStatus `json:"status"`
}

// RunResult is the response for GET /agent/run/{run_id}
type RunResult struct {
	// RunID is the run identifier
	RunID string `json:"run_id"`
	// TaskID is the client-provided task identifier
	TaskID string `json:"task_id"`
	// Status is the current run status
	Status RunStatus `json:"status"`
	// StartTime is when the run started
	StartTime time.Time `json:"start_time"`
	// EndTime is when the run ended (if completed)
	EndTime *time.Time `json:"end_time,omitempty"`
	// CoordinatorID is the AI coordinator ID
	CoordinatorID string `json:"coordinator_id,omitempty"`
	// Events contains the output events
	Events []*ypb.AIOutputEvent `json:"events,omitempty"`
	// Error contains error message if failed
	Error string `json:"error,omitempty"`
}

// CancelRunResponse is the response for POST /agent/run/{run_id}/cancel
type CancelRunResponse struct {
	// RunID is the run identifier
	RunID string `json:"run_id"`
	// Status is the new status after cancellation
	Status RunStatus `json:"status"`
	// Message is a human-readable message
	Message string `json:"message"`
}

// PushEventRequest is the request for GET /agent/run/{run_id}/events/push
type PushEventRequest struct {
	// Type is the event type
	Type string `json:"type"`
	// InteractiveID for interactive responses
	InteractiveID string `json:"interactive_id,omitempty"`
	// Input is the user input (JSON string)
	Input string `json:"input,omitempty"`
	// IsFreeInput indicates if this is a free-form input
	IsFreeInput bool `json:"is_free_input,omitempty"`
	// FreeInput is the free-form text input
	FreeInput string `json:"free_input,omitempty"`
}

// PushEventResponse is the response for push events
type PushEventResponse struct {
	// Success indicates if the push was successful
	Success bool `json:"success"`
	// Message is a human-readable message
	Message string `json:"message,omitempty"`
}

// SettingResponse wraps AIStartParams for JSON response
type SettingResponse struct {
	// Setting contains the current AI settings
	Setting *ypb.AIStartParams `json:"setting"`
}

// ErrorResponse is a standard error response
type ErrorResponse struct {
	// Error is the error type
	Error string `json:"error"`
	// Message is a human-readable error message
	Message string `json:"message"`
	// Code is an optional error code
	Code int `json:"code,omitempty"`
}

// SSEEvent represents a Server-Sent Event
type SSEEvent struct {
	// ID is the event ID
	ID string `json:"id,omitempty"`
	// Event is the event type
	Event string `json:"event,omitempty"`
	// Data is the event data (JSON)
	Data interface{} `json:"data"`
	// Retry is the retry interval in milliseconds
	Retry int `json:"retry,omitempty"`
}

// ConvertAIParamsToYPB converts AIParams to ypb.AIStartParams
func ConvertAIParamsToYPB(params *AIParams, base *ypb.AIStartParams) *ypb.AIStartParams {
	if base == nil {
		base = &ypb.AIStartParams{}
	}
	if params == nil {
		return base
	}

	result := &ypb.AIStartParams{
		UseDefaultAIConfig:             base.UseDefaultAIConfig,
		EnableSystemFileSystemOperator: base.EnableSystemFileSystemOperator,
	}

	if params.ForgeName != "" {
		result.ForgeName = params.ForgeName
	}
	if params.ReviewPolicy != "" {
		result.ReviewPolicy = params.ReviewPolicy
	}
	if params.AIService != "" {
		result.AIService = params.AIService
	}
	if params.DisableToolUse {
		result.DisableToolUse = true
	}
	if params.EnableAISearchTool {
		result.EnableAISearchTool = true
	}
	if params.ReActMaxIteration > 0 {
		result.ReActMaxIteration = params.ReActMaxIteration
	}
	if params.TimelineSessionID != "" {
		result.TimelineSessionID = params.TimelineSessionID
	}

	// Convert forge params
	if len(params.ForgeParams) > 0 {
		result.ForgeParams = make([]*ypb.ExecParamItem, 0, len(params.ForgeParams))
		for k, v := range params.ForgeParams {
			result.ForgeParams = append(result.ForgeParams, &ypb.ExecParamItem{
				Key:   k,
				Value: v,
			})
		}
	}

	return result
}
