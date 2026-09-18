package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	loopYaklangCodeStateKey           = "current_yaklang_code_state"
	loopYaklangCodeVersionKey         = "yaklang_code_change_version"
	loopYaklangCodeSourceActionKey    = "current_yaklang_code_source_action"
	loopYaklangCodeChangeReasonKey = "current_yaklang_code_change_reason"

	CodeEventOpReplace  = "replace"
	CodeEventOpSnapshot = "snapshot"
	CodeEventOpCreate   = "create"

	loopCodeEventOpReplace  = CodeEventOpReplace
	loopCodeEventOpSnapshot = CodeEventOpSnapshot
	loopCodeEventOpCreate   = CodeEventOpCreate
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

type loopYaklangCodeChangeResult struct {
	PreviousState *loopYaklangCodeState
	CurrentState  *loopYaklangCodeState
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

// IsLoopCodeSeededOnly reports whether full_code still equals the init seed and this loop
// has not committed a yaklang code change via applyLoopYaklangCodeChange yet.
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

// ResolvedYaklangCodeChangeVersion returns the committed yaklang code version (0 if none).
func ResolvedYaklangCodeChangeVersion(loop *reactloops.ReActLoop, fullCodeVar string) int {
	if loop == nil {
		return 0
	}
	_ = fullCodeVar
	return loop.GetInt(loopYaklangCodeVersionKey)
}

// HasCommittedYaklangCodeChange reports whether this loop committed code via write/modify/replace.
func HasCommittedYaklangCodeChange(loop *reactloops.ReActLoop, fullCodeVar string) bool {
	return ResolvedYaklangCodeChangeVersion(loop, fullCodeVar) > 0
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
	if loop.GetInt(loopYaklangCodeVersionKey) > 0 {
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

func resolveLoopYaklangCodeEventOp(explicitOp string, previousState *loopYaklangCodeState) (string, error) {
	switch strings.TrimSpace(explicitOp) {
	case "":
		if previousState == nil {
			return loopCodeEventOpCreate, nil
		}
		return loopCodeEventOpReplace, nil
	case loopCodeEventOpReplace, loopCodeEventOpCreate, loopCodeEventOpSnapshot:
		return strings.TrimSpace(explicitOp), nil
	default:
		return "", fmt.Errorf("unsupported code event op: %s", explicitOp)
	}
}

func (f *SingleFileModificationSuiteFactory) editorChangeEventType() schema.EventType {
	if f == nil {
		return ""
	}
	return f.editorDeliveryEventType
}

func (f *SingleFileModificationSuiteFactory) editorChangeEventNode() string {
	if f == nil {
		return ""
	}
	return f.editorDeliveryEventNode
}

func (f *SingleFileModificationSuiteFactory) editorChangeDefaultSource() string {
	if f == nil {
		return ""
	}
	return f.editorDeliverySource
}

// supportsYaklangCodeChangeEvent reports whether this suite should emit editor delivery events.
func (f *SingleFileModificationSuiteFactory) supportsYaklangCodeChangeEvent() bool {
	return f != nil && f.editorDeliveryEventType != ""
}

// applyLoopYaklangCodeChange updates loop file state and optionally emits an editor delivery event
// (yaklang_code_change or syntaxflow_rule_change).
func (f *SingleFileModificationSuiteFactory) applyLoopYaklangCodeChange(loop *reactloops.ReActLoop, input *loopYaklangCodeChange) (*loopYaklangCodeChangeResult, error) {
	if !f.supportsYaklangCodeChangeEvent() {
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

	fullCodeVar := f.GetFullCodeVariableName()
	filenameVar := f.GetFilenameVariableName()

	previousState := getLoopYaklangCodeState(loop, fullCodeVar, filenameVar)
	eventOp, err := resolveLoopYaklangCodeEventOp(input.EventOp, previousState)
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
	clearLoopCodeSeededOnly(loop)

	if input.DeliveryPatch != nil {
		SetLoopCodeDeliveryPatch(loop, input.DeliveryPatch)
	}

	if input.EmitEvent {
		f.emitLoopEditorChangeEvent(loop, currentState, eventOp)
	}

	return &loopYaklangCodeChangeResult{
		PreviousState: previousState,
		CurrentState:  cloneLoopYaklangCodeState(currentState),
	}, nil
}

// emitLoopEditorChangeEvent emits yaklang_code_change or syntaxflow_rule_change
// based on the suite content type (frontend routes on EventType).
func (f *SingleFileModificationSuiteFactory) emitLoopEditorChangeEvent(loop *reactloops.ReActLoop, state *loopYaklangCodeState, op string) {
	if f == nil || loop == nil || loop.GetEmitter() == nil || state == nil || strings.TrimSpace(state.Content) == "" {
		return
	}
	payload := BuildCodeFullChangeEvent(op, state.Path, state.Content, state.Version, state.SourceAction, state.ChangeReason, f.editorChangeDefaultSource())
	_, _ = loop.GetEmitter().EmitJSON(f.editorChangeEventType(), f.editorChangeEventNode(), payload)
}
