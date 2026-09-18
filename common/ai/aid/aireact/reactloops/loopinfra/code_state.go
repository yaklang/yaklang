package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	LoopCodeStateKey        = "current_code_state"
	LoopCodeVersionKey      = "code_change_version"
	LoopCodeSourceActionKey = "current_code_source_action"
	LoopCodeChangeReasonKey = "current_code_change_reason"

	CodeEventOpReplace  = "replace"
	CodeEventOpSnapshot = "snapshot"
	CodeEventOpCreate   = "create"
)

type loopCodeState struct {
	Content      string `json:"content"`
	Path         string `json:"path,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Version      int    `json:"version"`
	SourceAction string `json:"source_action,omitempty"`
	ChangeReason string `json:"change_reason,omitempty"`
}

// loopCodeChange is the in-memory editor commit applied by a single-file suite.
type loopCodeChange struct {
	Content      string
	Path         string
	SourceAction string
	ChangeReason string
	EventOp      string
	Version      int
	EmitEvent    bool
	// DeliveryPatch when set records a fragment delivery for editor_sync (live patch events).
	DeliveryPatch *CodeDeliveryPatch
}

type loopCodeChangeResult struct {
	PreviousState *loopCodeState
	CurrentState  *loopCodeState
}

func cloneLoopCodeState(state *loopCodeState) *loopCodeState {
	if state == nil {
		return nil
	}
	cloned := *state
	return &cloned
}

func getLoopCodeState(loop *reactloops.ReActLoop, fullCodeVar, filenameVar string) *loopCodeState {
	if loop == nil {
		return nil
	}

	switch state := loop.GetVariable(LoopCodeStateKey).(type) {
	case *loopCodeState:
		return cloneLoopCodeState(state)
	case loopCodeState:
		return cloneLoopCodeState(&state)
	}

	content := strings.TrimSpace(loop.Get(fullCodeVar))
	if content == "" {
		return nil
	}
	return &loopCodeState{
		Content:      content,
		Path:         strings.TrimSpace(loop.Get(filenameVar)),
		Summary:      buildLoopCodeSummary(content),
		Version:      max(loop.GetInt(LoopCodeVersionKey), 1),
		SourceAction: firstNonEmptyTrimmedString(loop.Get(LoopCodeSourceActionKey)),
		ChangeReason: firstNonEmptyTrimmedString(loop.Get(LoopCodeChangeReasonKey)),
	}
}

func buildLoopCodeSummary(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if len(content) > 200 {
		return content[:200] + "..."
	}
	return content
}

func firstNonEmptyTrimmedString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// IsLoopCodeSeededOnly reports whether full_code still equals the init seed and this loop
// has not committed a code change via applyLoopCodeChange yet.
func IsLoopCodeSeededOnly(loop *reactloops.ReActLoop) bool {
	if loop == nil {
		return false
	}
	switch v := loop.GetVariable(LoopVarCodeSeededOnly).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}

func isLoopCodeSeededOnly(loop *reactloops.ReActLoop) bool {
	return IsLoopCodeSeededOnly(loop)
}

// ResolvedCodeChangeVersion returns the committed editor-change version (0 if none).
func ResolvedCodeChangeVersion(loop *reactloops.ReActLoop, fullCodeVar string) int {
	if loop == nil {
		return 0
	}
	_ = fullCodeVar
	return loop.GetInt(LoopCodeVersionKey)
}

// HasCommittedCodeChange reports whether this loop committed code via write/modify/replace.
func HasCommittedCodeChange(loop *reactloops.ReActLoop, fullCodeVar string) bool {
	return ResolvedCodeChangeVersion(loop, fullCodeVar) > 0
}

func clearLoopCodeSeededOnly(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(LoopVarCodeSeededOnly, false)
}

// AllowWriteCodeDespiteExistingSeed returns true when full_code was seeded from an external
// file (another session or disk) and this loop has not committed code changes yet.
func AllowWriteCodeDespiteExistingSeed(loop *reactloops.ReActLoop, fullCodeVar string) bool {
	if loop == nil || !isLoopCodeSeededOnly(loop) {
		return false
	}
	if loop.GetInt(LoopCodeVersionKey) > 0 {
		return false
	}
	existing := strings.TrimSpace(loop.Get(fullCodeVar))
	if existing == "" {
		return false
	}
	seed := strings.TrimSpace(loop.Get(LoopVarInitSeedFullCode))
	if seed == "" {
		seed = existing
	}
	return existing == seed
}

func resolveLoopCodeEventOp(explicitOp string, previousState *loopCodeState) (string, error) {
	switch strings.TrimSpace(explicitOp) {
	case "":
		if previousState == nil {
			return CodeEventOpCreate, nil
		}
		return CodeEventOpReplace, nil
	case CodeEventOpReplace, CodeEventOpCreate, CodeEventOpSnapshot:
		return strings.TrimSpace(explicitOp), nil
	default:
		return "", fmt.Errorf("unsupported code event op: %s", explicitOp)
	}
}

func (f *SingleFileModificationSuiteFactory) supportsEditorChangeEvent() bool {
	return f != nil && f.editorChange != ""
}

func (f *SingleFileModificationSuiteFactory) emitEditorStreamJSON(loop *reactloops.ReActLoop, nodeId string, payload any) {
	if f == nil || loop == nil || loop.GetEmitter() == nil {
		return
	}
	eventType := strings.TrimSpace(f.eventType)
	if eventType == "" {
		return
	}
	loop.GetEmitter().EmitJSON(schema.EventType(eventType), nodeId, payload)
}

// applyLoopCodeChange updates loop file state and optionally emits an editor change event
// configured by WithEditorChange.
func (f *SingleFileModificationSuiteFactory) applyLoopCodeChange(loop *reactloops.ReActLoop, input *loopCodeChange) (*loopCodeChangeResult, error) {
	if !f.supportsEditorChangeEvent() {
		return nil, nil
	}
	if loop == nil {
		return nil, fmt.Errorf("loop is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("code change input is nil")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, fmt.Errorf("code content cannot be empty")
	}
	if strings.TrimSpace(input.SourceAction) == "" {
		return nil, fmt.Errorf("source action cannot be empty")
	}

	fullCodeVar := f.GetFullCodeVariableName()
	filenameVar := f.GetFilenameVariableName()

	previousState := getLoopCodeState(loop, fullCodeVar, filenameVar)
	eventOp, err := resolveLoopCodeEventOp(input.EventOp, previousState)
	if err != nil {
		return nil, err
	}
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
	currentState := &loopCodeState{
		Content:      content,
		Path:         path,
		Summary:      buildLoopCodeSummary(content),
		Version:      version,
		SourceAction: strings.TrimSpace(input.SourceAction),
		ChangeReason: strings.TrimSpace(input.ChangeReason),
	}

	loop.Set(fullCodeVar, content)
	if path != "" {
		loop.Set(filenameVar, path)
	}
	loop.Set(LoopCodeStateKey, *currentState)
	loop.Set(LoopCodeVersionKey, currentState.Version)
	loop.Set(LoopCodeSourceActionKey, currentState.SourceAction)
	loop.Set(LoopCodeChangeReasonKey, currentState.ChangeReason)
	clearLoopCodeSeededOnly(loop)

	if input.DeliveryPatch != nil {
		SetLoopCodeDeliveryPatch(loop, input.DeliveryPatch)
	}

	if input.EmitEvent {
		f.emitLoopEditorChangeEvent(loop, currentState, eventOp)
	}

	return &loopCodeChangeResult{
		PreviousState: previousState,
		CurrentState:  cloneLoopCodeState(currentState),
	}, nil
}

// emitLoopEditorChangeEvent emits the suite's configured editor-change event.
func (f *SingleFileModificationSuiteFactory) emitLoopEditorChangeEvent(loop *reactloops.ReActLoop, state *loopCodeState, op string) {
	if f == nil || loop == nil || loop.GetEmitter() == nil || state == nil || strings.TrimSpace(state.Content) == "" {
		return
	}
	eventType := f.editorChange
	payload := BuildCodeFullChangeEvent(op, state.Path, state.Content, state.Version, state.SourceAction, state.ChangeReason, "")
	_, _ = loop.GetEmitter().EmitJSON(eventType, string(eventType), payload)
}
