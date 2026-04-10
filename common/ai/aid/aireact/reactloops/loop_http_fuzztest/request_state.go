package loop_http_fuzztest

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	loopHTTPFuzzRequestStateKey        = "current_request_state"
	loopHTTPFuzzRequestVersionKey      = "current_request_version"
	loopHTTPFuzzRequestSourceActionKey = "current_request_source_action"
	loopHTTPFuzzRequestChangeReasonKey = "current_request_change_reason"
	loopHTTPFuzzRequestChangeEventNode = "http_fuzz_request_change"
	loopHTTPFuzzRequestEventOpSnapshot = "snapshot"
	loopHTTPFuzzRequestEventOpReplace  = "replace"
	loopHTTPFuzzRequestEventOpPatch    = "patch_applied"
)

type loopHTTPFuzzRequestState struct {
	RawRequest   string `json:"raw_request"`
	IsHTTPS      bool   `json:"is_https"`
	Summary      string `json:"summary"`
	Version      int    `json:"version"`
	SourceAction string `json:"source_action,omitempty"`
	ChangeReason string `json:"change_reason,omitempty"`
}

type loopHTTPFuzzRequestChange struct {
	RawRequest          string
	IsHTTPS             bool
	SourceAction        string
	ChangeReason        string
	ReviewDecision      string
	EventOp             string
	PersistSource       string
	Task                aicommon.AIStatefulTask
	Version             int
	ResetBaseline       bool
	ClearActionTracking bool
	EmitEvent           bool
	EmitEditablePacket  bool
	PersistSession      bool
}

type loopHTTPFuzzRequestChangeResult struct {
	PreviousState *loopHTTPFuzzRequestState
	CurrentState  *loopHTTPFuzzRequestState
	Diff          string
}

type loopHTTPFuzzRequestChangeEvent struct {
	Op           string                               `json:"op"`
	Request      loopHTTPFuzzRequestChangeEventPacket `json:"request"`
	Reason       string                               `json:"reason,omitempty"`
	SourceAction string                               `json:"source_action,omitempty"`
}

type loopHTTPFuzzRequestChangeEventPacket struct {
	Raw     string `json:"raw"`
	IsHTTPS bool   `json:"is_https"`
	Summary string `json:"summary"`
	Version int    `json:"version"`
}

func cloneLoopHTTPFuzzRequestState(state *loopHTTPFuzzRequestState) *loopHTTPFuzzRequestState {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

func getLoopHTTPFuzzRequestState(loop *reactloops.ReActLoop) *loopHTTPFuzzRequestState {
	if loop == nil {
		return nil
	}

	switch state := loop.GetVariable(loopHTTPFuzzRequestStateKey).(type) {
	case *loopHTTPFuzzRequestState:
		return cloneLoopHTTPFuzzRequestState(state)
	case loopHTTPFuzzRequestState:
		return cloneLoopHTTPFuzzRequestState(&state)
	}

	rawRequest := strings.TrimSpace(loop.Get("current_request"))
	if rawRequest == "" {
		rawRequest = strings.TrimSpace(loop.Get("original_request"))
	}
	if strings.TrimSpace(rawRequest) == "" {
		return nil
	}
	summary := strings.TrimSpace(loop.Get("current_request_summary"))
	if summary == "" {
		summary = strings.TrimSpace(loop.Get("original_request_summary"))
	}

	return &loopHTTPFuzzRequestState{
		RawRequest:   rawRequest,
		IsHTTPS:      strings.EqualFold(loop.Get("is_https"), "true"),
		Summary:      summary,
		Version:      max(loop.GetInt(loopHTTPFuzzRequestVersionKey), 1),
		SourceAction: firstNonEmptyString(loop.Get(loopHTTPFuzzRequestSourceActionKey), loop.Get("bootstrap_source")),
		ChangeReason: firstNonEmptyString(loop.Get(loopHTTPFuzzRequestChangeReasonKey), loop.Get("request_modification_reason")),
	}
}

func applyLoopHTTPFuzzRequestChange(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime, input *loopHTTPFuzzRequestChange) (*loopHTTPFuzzRequestChangeResult, error) {
	if loop == nil {
		return nil, fmt.Errorf("loop is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("request change input is nil")
	}
	if strings.TrimSpace(input.RawRequest) == "" {
		return nil, fmt.Errorf("raw request cannot be empty")
	}
	if strings.TrimSpace(input.SourceAction) == "" {
		return nil, fmt.Errorf("source action cannot be empty")
	}

	eventOp := strings.TrimSpace(input.EventOp)
	switch eventOp {
	case "", loopHTTPFuzzRequestEventOpReplace:
		eventOp = loopHTTPFuzzRequestEventOpReplace
	case loopHTTPFuzzRequestEventOpSnapshot, loopHTTPFuzzRequestEventOpPatch:
	default:
		return nil, fmt.Errorf("unsupported request event op: %s", eventOp)
	}

	fixedPacket := lowhttp.FixHTTPRequest([]byte(input.RawRequest))
	if len(bytes.TrimSpace(fixedPacket)) == 0 {
		return nil, fmt.Errorf("fixed HTTP request is empty")
	}

	fuzzReq, err := newLoopFuzzRequest(getLoopRequestTaskContext(loop, input.Task), runtime, fixedPacket, input.IsHTTPS)
	if err != nil {
		return nil, fmt.Errorf("failed to create fuzz request: %w", err)
	}

	previousState := getLoopHTTPFuzzRequestState(loop)
	version := input.Version
	if version <= 0 {
		if previousState != nil {
			version = previousState.Version + 1
		} else {
			version = 1
		}
	}

	_, summary := buildHTTPRequestStreamSummary(string(fixedPacket), input.IsHTTPS)
	currentState := &loopHTTPFuzzRequestState{
		RawRequest:   string(fixedPacket),
		IsHTTPS:      input.IsHTTPS,
		Summary:      summary,
		Version:      version,
		SourceAction: strings.TrimSpace(input.SourceAction),
		ChangeReason: strings.TrimSpace(input.ChangeReason),
	}

	diffSummary := ""
	if previousState != nil && strings.TrimSpace(previousState.RawRequest) != "" && !input.ResetBaseline {
		diffSummary = compareRequests(previousState.RawRequest, currentState.RawRequest)
	}

	loop.Set("fuzz_request", fuzzReq)
	loop.Set(loopHTTPFuzzRequestStateKey, *currentState)
	loop.Set(loopHTTPFuzzRequestVersionKey, currentState.Version)
	loop.Set(loopHTTPFuzzRequestSourceActionKey, currentState.SourceAction)
	loop.Set(loopHTTPFuzzRequestChangeReasonKey, currentState.ChangeReason)
	loop.Set("current_request", currentState.RawRequest)
	loop.Set("current_request_summary", currentState.Summary)
	loop.Set("is_https", utils.InterfaceToString(currentState.IsHTTPS))
	loop.Set("bootstrap_source", currentState.SourceAction)
	loop.Set("request_modification_reason", currentState.ChangeReason)
	loop.Set("request_review_decision", strings.TrimSpace(input.ReviewDecision))

	if input.ResetBaseline || strings.TrimSpace(loop.Get("original_request")) == "" {
		loop.Set("original_request", currentState.RawRequest)
		loop.Set("original_request_summary", currentState.Summary)
		loop.Set("previous_request", "")
		loop.Set("previous_request_summary", "")
		loop.Set("request_change_summary", "")
		resetLoopHTTPFuzzExecutionState(loop)
		if input.ClearActionTracking {
			clearLoopHTTPFuzzActionTracking(loop)
		}
	} else {
		previousRaw := ""
		previousSummary := ""
		if previousState != nil {
			previousRaw = previousState.RawRequest
			previousSummary = previousState.Summary
		}
		loop.Set("previous_request", previousRaw)
		loop.Set("previous_request_summary", previousSummary)
		loop.Set("request_change_summary", diffSummary)
	}

	if input.EmitEvent {
		emitLoopHTTPFuzzRequestChangeEvent(loop, currentState, eventOp)
	}
	if input.EmitEditablePacket {
		emitLoopHTTPFuzzEditablePacket(loop, input.Task, currentState.RawRequest)
	}
	if input.PersistSession {
		persistSource := firstNonEmptyString(input.PersistSource, input.SourceAction)
		persistLoopHTTPFuzzSessionContext(loop, persistSource)
	}

	return &loopHTTPFuzzRequestChangeResult{
		PreviousState: previousState,
		CurrentState:  cloneLoopHTTPFuzzRequestState(currentState),
		Diff:          diffSummary,
	}, nil
}

func emitLoopHTTPFuzzRequestChangeEvent(loop *reactloops.ReActLoop, state *loopHTTPFuzzRequestState, op string) {
	if loop == nil || loop.GetEmitter() == nil || state == nil || strings.TrimSpace(state.RawRequest) == "" {
		return
	}
	if op == "" {
		op = loopHTTPFuzzRequestEventOpReplace
	}
	_, _ = loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_HTTP_FUZZ_REQUEST_CHANGE, loopHTTPFuzzRequestChangeEventNode, loopHTTPFuzzRequestChangeEvent{
		Op: op,
		Request: loopHTTPFuzzRequestChangeEventPacket{
			Raw:     state.RawRequest,
			IsHTTPS: state.IsHTTPS,
			Summary: state.Summary,
			Version: state.Version,
		},
		Reason:       state.ChangeReason,
		SourceAction: state.SourceAction,
	})
}

func emitLoopHTTPFuzzEditablePacket(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, rawPacket string) {
	if loop == nil || loop.GetEmitter() == nil || strings.TrimSpace(rawPacket) == "" {
		return
	}
	if task == nil {
		task = loop.GetCurrentTask()
	}
	if task == nil {
		return
	}
	taskID := task.GetId()
	if taskID == "" {
		taskID = utils.InterfaceToString(task.GetIndex())
	}
	if taskID == "" {
		return
	}
	_, _ = loop.GetEmitter().EmitHTTPRequestStreamEvent("http_flow", strings.NewReader(rawPacket), taskID)
}

func getLoopRequestTaskContext(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) context.Context {
	if task != nil {
		return task.GetContext()
	}
	return getLoopTaskContext(loop)
}

func resetLoopHTTPFuzzExecutionState(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set("last_request", "")
	loop.Set("last_request_summary", "")
	loop.Set("last_response", "")
	loop.Set("last_response_summary", "")
	loop.Set("last_httpflow_hidden_index", "")
	loop.Set("representative_request", "")
	loop.Set("representative_response", "")
	loop.Set("representative_httpflow_hidden_index", "")
	loop.Set("diff_result", "")
	loop.Set("diff_result_full", "")
	loop.Set("diff_result_compressed", "")
	loop.Set("verification_result", "")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
