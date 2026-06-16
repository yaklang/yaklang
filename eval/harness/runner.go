package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TaskConfig holds parameters for an AI Agent evaluation task.
type TaskConfig struct {
	Model             string
	AIService         string
	UserQuery         string
	FocusMode         string
	ProgramName       string
	ScanTargetPath    string // Absolute path to the project root for code_security_audit
	ReActMaxIteration int
	ReviewPolicy      string
	// SessionID is a unique ID for memory isolation between eval runs.
	// Each run should use a fresh UUID-based session ID.
	SessionID string
}

// TaskResult holds the outcome of a single AI Agent evaluation task.
type TaskResult struct {
	CoordinatorID string               `json:"coordinator_id"`
	StartTime     time.Time            `json:"start_time"`
	EndTime       time.Time            `json:"end_time"`
	Duration      time.Duration        `json:"duration"`
	Events        []*ypb.AIOutputEvent `json:"-"`
	EventCount    int                  `json:"event_count"`
	ThoughtCount  int                  `json:"thought_count"`
	ToolCallCount int                  `json:"tool_call_count"`
	ErrorCount    int                  `json:"error_count"`
	ErrorMessages []string             `json:"error_messages"`
	FinalAnswer   string               `json:"final_answer"`
	StreamErrors  []string             `json:"stream_errors"`
	TokenUsage    TokenUsage           `json:"token_usage"`
	// consumptionAccumulated tracks authoritative input/output tokens from aibalance consumption events.
	consumptionAccumulated TokenUsage
}

// IsThoughtEvent detects reasoning events from both live stream and exported logs.
// Live stream may use Type="thought"; exported logs use Type="stream" with NodeId="re-act-loop-thought".
func IsThoughtEvent(e *ypb.AIOutputEvent) bool {
	if e == nil {
		return false
	}
	if e.Type == "thought" {
		return true
	}
	if e.Type == "stream" && e.NodeId == "re-act-loop-thought" {
		return true
	}
	return false
}

// IsToolCallEvent detects tool invocation events from both live stream and exported logs.
// Live stream may use Type="call_tool" or "tool_call_start"; exported logs encode tool calls as
// Type="structured" NodeId="timeline_item" with entry_type="tool_call".
func IsToolCallEvent(e *ypb.AIOutputEvent) bool {
	if e == nil {
		return false
	}
	if e.Type == "call_tool" || e.Type == "tool_call_start" {
		return true
	}
	if e.Type == "structured" && e.NodeId == "timeline_item" {
		entryType := gjson.GetBytes(e.Content, "entry_type").String()
		if entryType == "tool_call" {
			return true
		}
	}
	return false
}

// IsErrorEvent detects error events.
func IsErrorEvent(e *ypb.AIOutputEvent) bool {
	return e != nil && e.Type == "error"
}

// ExtractTimelineEntryType returns the entry_type field for timeline_item structured events.
func ExtractTimelineEntryType(e *ypb.AIOutputEvent) string {
	if e == nil || e.Type != "structured" || e.NodeId != "timeline_item" {
		return ""
	}
	return gjson.GetBytes(e.Content, "entry_type").String()
}

// RecomputeStats recalculates counts from collected events.
// Useful when TaskResult is rebuilt from exported logs or QueryAIEvent results.
func (r *TaskResult) RecomputeStats() {
	r.EventCount = len(r.Events)
	r.ThoughtCount = 0
	r.ToolCallCount = 0
	r.ErrorCount = 0
	r.ErrorMessages = r.ErrorMessages[:0]
	r.TokenUsage = TokenUsage{}
	for _, e := range r.Events {
		if IsThoughtEvent(e) {
			r.ThoughtCount++
		}
		if IsToolCallEvent(e) {
			r.ToolCallCount++
		}
		if IsErrorEvent(e) {
			r.ErrorCount++
			r.ErrorMessages = append(r.ErrorMessages, string(e.Content))
		}
		if e.Type == "structured" && e.NodeId == "result" {
			r.FinalAnswer = string(e.Content)
		}
		usage := EstimateEventTokens(e.Content, e.StreamDelta, e.Type, e.NodeId)
		r.TokenUsage.InputTokens += usage.InputTokens
		r.TokenUsage.OutputTokens += usage.OutputTokens
		r.TokenUsage.TotalTokens += usage.TotalTokens

		if inTok, outTok, ok := ParseConsumptionEvent(e); ok {
			if inTok > r.consumptionAccumulated.InputTokens {
				r.consumptionAccumulated.InputTokens = inTok
			}
			if outTok > r.consumptionAccumulated.OutputTokens {
				r.consumptionAccumulated.OutputTokens = outTok
			}
			r.consumptionAccumulated.TotalTokens = r.consumptionAccumulated.InputTokens + r.consumptionAccumulated.OutputTokens
		}
	}

	// Prefer authoritative consumption totals if they exceed the heuristic estimate.
	if r.consumptionAccumulated.Total() > r.TokenUsage.Total() {
		r.TokenUsage = r.consumptionAccumulated
	}
}

// RunTask starts an AI Agent task via StartAIReAct and collects all events.
// Using StartAIReAct (simpler) instead of StartAITask (coordinator with plan/verify loops).
func RunTask(ctx context.Context, client *Client, cfg TaskConfig) (*TaskResult, error) {
	if cfg.ReviewPolicy == "" {
		cfg.ReviewPolicy = "yolo"
	}
	if cfg.ReActMaxIteration == 0 {
		cfg.ReActMaxIteration = 30
	}

	// Use a separate context with longer timeout for the AI task.
	taskCtx, taskCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer taskCancel()

	stream, err := client.Raw().StartAIReAct(taskCtx)
	if err != nil {
		return nil, fmt.Errorf("StartAIReAct: %w", err)
	}

	// Generate a coordinator ID for this task.
	coordinatorID := uuid.New().String()

	startParams := &ypb.AIStartParams{
		UserQuery:                      cfg.UserQuery,
		AIService:                      cfg.AIService,
		AIModelName:                    cfg.Model,
		ReActMaxIteration:              int64(cfg.ReActMaxIteration),
		ReviewPolicy:                   cfg.ReviewPolicy,
		UseDefaultAIConfig:             cfg.AIService == "",
		CoordinatorId:                  coordinatorID,
		EnableSystemFileSystemOperator: true,
		TimelineSessionID:              cfg.SessionID,
	}
	if err := stream.Send(&ypb.AIInputEvent{
		IsStart: true,
		Params:  startParams,
	}); err != nil {
		return nil, fmt.Errorf("send start event: %w", err)
	}

	// Build the FreeInput event with focus mode and attached resources.
	freeInputEvent := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   cfg.UserQuery,
	}
	// Set focus mode via the proper field (not via @__FOCUS__ query prefix).
	if cfg.FocusMode != "" {
		freeInputEvent.FocusModeLoop = cfg.FocusMode
	}
	// Attach scan target path for code_security_audit focus mode.
	if cfg.ScanTargetPath != "" {
		freeInputEvent.AttachedResourceInfo = []*ypb.AttachedResourceInfo{
			{
				Type:  "file",
				Key:   "code_audit_target_path",
				Value: cfg.ScanTargetPath,
			},
		}
	}
	if err := stream.Send(freeInputEvent); err != nil {
		return nil, fmt.Errorf("send free input event: %w", err)
	}

	result := &TaskResult{
		CoordinatorID: coordinatorID,
		StartTime:     time.Now(),
		Events:        make([]*ypb.AIOutputEvent, 0),
		ErrorMessages: make([]string, 0),
		StreamErrors:  make([]string, 0),
	}

	fmt.Printf("[harness] Task started, coordinator=%s\n", coordinatorID)

	// Receive loop.
	recvCount := 0
	for {
		event, err := stream.Recv()
		if err != nil {
			errStr := err.Error()
			fmt.Printf("[harness] Stream ended: %s\n", errStr)
			result.StreamErrors = append(result.StreamErrors, errStr)
			break
		}

		result.Events = append(result.Events, event)
		result.EventCount++
		recvCount++

		// Log event type periodically.
		isImportant := event.Type == "structured" || event.Type == "error" ||
			IsThoughtEvent(event) || IsToolCallEvent(event)
		if recvCount%20 == 0 || isImportant {
			contentPreview := ""
			if len(event.Content) > 0 && len(event.Content) < 200 {
				contentPreview = string(event.Content)
			} else if len(event.Content) >= 200 {
				contentPreview = string(event.Content[:200]) + "..."
			}
			entryType := ""
			if event.Type == "structured" && event.NodeId == "timeline_item" {
				entryType = fmt.Sprintf(" entry=%s", ExtractTimelineEntryType(event))
			}
			fmt.Printf("[harness] Event #%d type=%s node=%s%s content=%s\n",
				recvCount, event.Type, event.NodeId, entryType, contentPreview)
		}

		if IsThoughtEvent(event) {
			result.ThoughtCount++
		}
		if IsToolCallEvent(event) {
			result.ToolCallCount++
		}
		if IsErrorEvent(event) {
			result.ErrorCount++
			result.ErrorMessages = append(result.ErrorMessages, string(event.Content))
		}
		if event.Type == "structured" && event.NodeId == "result" {
			result.FinalAnswer = string(event.Content)
		}
		usage := EstimateEventTokens(event.Content, event.StreamDelta, event.Type, event.NodeId)
		result.TokenUsage.InputTokens += usage.InputTokens
		result.TokenUsage.OutputTokens += usage.OutputTokens
		result.TokenUsage.TotalTokens += usage.TotalTokens

		// Track authoritative consumption events from aibalance.
		if inTok, outTok, ok := ParseConsumptionEvent(event); ok {
			if inTok > result.consumptionAccumulated.InputTokens {
				result.consumptionAccumulated.InputTokens = inTok
			}
			if outTok > result.consumptionAccumulated.OutputTokens {
				result.consumptionAccumulated.OutputTokens = outTok
			}
			result.consumptionAccumulated.TotalTokens = result.consumptionAccumulated.InputTokens + result.consumptionAccumulated.OutputTokens
		}

		// Auto-approve review requests in yolo mode.
		if cfg.ReviewPolicy == "yolo" {
			if event.Type == "task_review_require" ||
				event.Type == "plan_review_require" ||
				event.Type == "tool_use_review_require" {
				interactiveId := gjson.GetBytes(event.Content, "id").String()
				fmt.Printf("[harness] Auto-approving review: %s (type=%s)\n", interactiveId, event.Type)
				stream.Send(&ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        interactiveId,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				})
			}
		}

		// Capture coordinator ID from events if different.
		if event.CoordinatorId != "" && event.CoordinatorId != coordinatorID {
			result.CoordinatorID = event.CoordinatorId
		}

		// Check for completion.
		if event.Type == "structured" && event.NodeId == "result" {
			fmt.Printf("[harness] Task completed with final answer\n")
			break
		}
		if event.Type == "success_react_task" {
			fmt.Printf("[harness] ReAct task completed successfully\n")
			break
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Prefer authoritative consumption totals if they exceed the heuristic estimate.
	if result.consumptionAccumulated.Total() > result.TokenUsage.Total() {
		result.TokenUsage = result.consumptionAccumulated
	}

	fmt.Printf("[harness] Task finished: %d events, %d thoughts, %d tool calls, %d errors, %.1fs, ~%d tokens\n",
		result.EventCount, result.ThoughtCount, result.ToolCallCount, result.ErrorCount, result.Duration.Seconds(), result.TokenUsage.TotalTokens)

	return result, nil
}

// QueryEvents retrieves all persisted AI events for a coordinator.
func QueryEvents(ctx context.Context, client *Client, coordinatorID string) ([]*ypb.AIOutputEvent, error) {
	resp, err := client.Raw().QueryAIEvent(ctx, &ypb.AIEventQueryRequest{
		Filter: &ypb.AIEventFilter{
			CoordinatorId: []string{coordinatorID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("QueryAIEvent: %w", err)
	}
	return resp.GetEvents(), nil
}

// ExportLogs exports AI session logs as a ZIP file.
func ExportLogs(ctx context.Context, client *Client, coordinatorID string, outputPath string) (string, error) {
	resp, err := client.Raw().ExportAILogs(ctx, &ypb.ExportAILogsRequest{
		CoordinatorIDs: []string{coordinatorID},
		ExportDataTypes: []string{
			"checkpoints",
			"output_event",
			"memory",
			"timeline",
		},
		OutputPath: outputPath,
	})
	if err != nil {
		return "", fmt.Errorf("ExportAILogs: %w", err)
	}
	return resp.FilePath, nil
}

// EventsToJSON serializes events to indented JSON.
func EventsToJSON(events []*ypb.AIOutputEvent) ([]byte, error) {
	return json.MarshalIndent(events, "", "  ")
}

// SaveEventsJSON writes events to a JSON file.
// This captures the full live stream (including events that ExportAILogs may omit).
func SaveEventsJSON(events []*ypb.AIOutputEvent, outputPath string) error {
	data, err := EventsToJSON(events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(outputPath), err)
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", outputPath, err)
	}
	return nil
}
