package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	loopYaklangCodeStateKey        = "current_yaklang_code_state"
	loopYaklangCodeVersionKey      = "yaklang_code_change_version"
	loopYaklangCodeSourceActionKey = "current_yaklang_code_source_action"
	loopYaklangCodeChangeReasonKey = "current_yaklang_code_change_reason"
	loopYaklangCodeChangeEventNode = "yaklang_code_change"
	loopYaklangCodeEventOpReplace  = "replace"
	loopYaklangCodeEventOpSnapshot = "snapshot"
)

type loopYaklangCodeState struct {
	Content      string `json:"content"`
	Path         string `json:"path,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Version      int    `json:"version"`
	SourceAction string `json:"source_action,omitempty"`
	ChangeReason string `json:"change_reason,omitempty"`
}

// loopYaklangCodeChange mirrors loop_http_fuzztest.loopHTTPFuzzRequestChange.
type loopYaklangCodeChange struct {
	Content       string
	Path          string
	SourceAction  string
	ChangeReason  string
	EventOp       string
	Version       int
	ResetBaseline bool
	EmitEvent     bool
}

type loopYaklangCodeChangeResult struct {
	PreviousState *loopYaklangCodeState
	CurrentState  *loopYaklangCodeState
}

type loopYaklangCodeChangeEvent struct {
	Op           string                         `json:"op"`
	Code         loopYaklangCodeChangeEventCode `json:"code"`
	Reason       string                         `json:"reason,omitempty"`
	SourceAction string                         `json:"source_action,omitempty"`
}

type loopYaklangCodeChangeEventCode struct {
	Content string `json:"content"`
	Path    string `json:"path,omitempty"`
	Summary string `json:"summary,omitempty"`
	Version int    `json:"version"`
}

func cloneLoopYaklangCodeState(state *loopYaklangCodeState) *loopYaklangCodeState {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

func getLoopYaklangCodeState(loop *reactloops.ReActLoop, fullCodeVar, filenameVar string) *loopYaklangCodeState {
	if loop == nil {
		return nil
	}

	switch state := loop.GetVariable(loopYaklangCodeStateKey).(type) {
	case *loopYaklangCodeState:
		return cloneLoopYaklangCodeState(state)
	case loopYaklangCodeState:
		return cloneLoopYaklangCodeState(&state)
	}

	content := strings.TrimSpace(loop.Get(fullCodeVar))
	if content == "" {
		return nil
	}
	return &loopYaklangCodeState{
		Content:      content,
		Path:         strings.TrimSpace(loop.Get(filenameVar)),
		Summary:      buildLoopYaklangCodeSummary(content),
		Version:      max(loop.GetInt(loopYaklangCodeVersionKey), 1),
		SourceAction: firstNonEmptyYaklangString(loop.Get(loopYaklangCodeSourceActionKey)),
		ChangeReason: firstNonEmptyYaklangString(loop.Get(loopYaklangCodeChangeReasonKey)),
	}
}

func buildLoopYaklangCodeSummary(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if len(content) > 200 {
		return content[:200] + "..."
	}
	return content
}

func firstNonEmptyYaklangString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// applyLoopYaklangCodeChange updates loop file state and optionally emits yaklang_code_change (same pattern as http_fuzz_request_change).
func (f *SingleFileModificationSuiteFactory) applyLoopYaklangCodeChange(loop *reactloops.ReActLoop, input *loopYaklangCodeChange) (*loopYaklangCodeChangeResult, error) {
	if f.contentType != "code/yaklang" {
		return nil, nil
	}
	if loop == nil {
		return nil, fmt.Errorf("loop is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("yaklang code change input is nil")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, fmt.Errorf("yaklang code content cannot be empty")
	}
	if strings.TrimSpace(input.SourceAction) == "" {
		return nil, fmt.Errorf("source action cannot be empty")
	}

	eventOp := strings.TrimSpace(input.EventOp)
	switch eventOp {
	case "", loopYaklangCodeEventOpReplace:
		eventOp = loopYaklangCodeEventOpReplace
	case loopYaklangCodeEventOpSnapshot:
	default:
		return nil, fmt.Errorf("unsupported yaklang code event op: %s", eventOp)
	}

	fullCodeVar := f.GetFullCodeVariableName()
	filenameVar := f.GetFilenameVariableName()

	previousState := getLoopYaklangCodeState(loop, fullCodeVar, filenameVar)
	version := input.Version
	if version <= 0 {
		if previousState != nil {
			version = previousState.Version + 1
		} else {
			version = 1
		}
	}

	path := strings.TrimSpace(input.Path)
	if path == "" && previousState != nil {
		path = previousState.Path
	}
	if path == "" {
		path = strings.TrimSpace(loop.Get(filenameVar))
	}

	content := input.Content
	currentState := &loopYaklangCodeState{
		Content:      content,
		Path:         path,
		Summary:      buildLoopYaklangCodeSummary(content),
		Version:      version,
		SourceAction: strings.TrimSpace(input.SourceAction),
		ChangeReason: strings.TrimSpace(input.ChangeReason),
	}

	loop.Set(fullCodeVar, content)
	if path != "" {
		loop.Set(filenameVar, path)
	}
	loop.Set(loopYaklangCodeStateKey, *currentState)
	loop.Set(loopYaklangCodeVersionKey, currentState.Version)
	loop.Set(loopYaklangCodeSourceActionKey, currentState.SourceAction)
	loop.Set(loopYaklangCodeChangeReasonKey, currentState.ChangeReason)

	if input.EmitEvent {
		emitLoopYaklangCodeChangeEvent(loop, currentState, eventOp)
	}

	return &loopYaklangCodeChangeResult{
		PreviousState: previousState,
		CurrentState:  cloneLoopYaklangCodeState(currentState),
	}, nil
}

func emitLoopYaklangCodeChangeEvent(loop *reactloops.ReActLoop, state *loopYaklangCodeState, op string) {
	if loop == nil || loop.GetEmitter() == nil || state == nil || strings.TrimSpace(state.Content) == "" {
		return
	}
	if op == "" {
		op = loopYaklangCodeEventOpReplace
	}
	_, _ = loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE, loopYaklangCodeChangeEventNode, loopYaklangCodeChangeEvent{
		Op: op,
		Code: loopYaklangCodeChangeEventCode{
			Content: state.Content,
			Path:    state.Path,
			Summary: state.Summary,
			Version: state.Version,
		},
		Reason:       state.ChangeReason,
		SourceAction: state.SourceAction,
	})
}
