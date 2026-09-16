package loopinfra

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	loopSyntaxflowRuleChangeEventNode = "syntaxflow_rule_change"
	contentTypeSyntaxFlowRule         = "text/syntaxflow"
)

func isSyntaxFlowRuleEditorContentType(contentType string) bool {
	return contentType == contentTypeSyntaxFlowRule
}

func (f *SingleFileModificationSuiteFactory) editorChangeEventType() schema.EventType {
	if f != nil && isSyntaxFlowRuleEditorContentType(f.contentType) {
		return schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE
	}
	return schema.EVENT_TYPE_YAKLANG_CODE_CHANGE
}

func (f *SingleFileModificationSuiteFactory) editorChangeEventNode() string {
	if f != nil && isSyntaxFlowRuleEditorContentType(f.contentType) {
		return loopSyntaxflowRuleChangeEventNode
	}
	return loopYaklangCodeChangeEventNode
}

// emitLoopEditorChangeEvent emits yaklang_code_change or syntaxflow_rule_change
// based on the suite content type (frontend routes on EventType).
func (f *SingleFileModificationSuiteFactory) emitLoopEditorChangeEvent(loop *reactloops.ReActLoop, state *loopYaklangCodeState, op string) {
	if loop == nil || loop.GetEmitter() == nil || state == nil || strings.TrimSpace(state.Content) == "" {
		return
	}
	if isSyntaxFlowRuleEditorContentType(f.contentType) {
		emitLoopSyntaxFlowRuleChangeEvent(loop, state, op)
		return
	}
	emitLoopYaklangCodeChangeEvent(loop, state, op)
}

func emitLoopSyntaxFlowRuleChangeEvent(loop *reactloops.ReActLoop, state *loopYaklangCodeState, op string) {
	if loop == nil || loop.GetEmitter() == nil || state == nil {
		return
	}
	if strings.TrimSpace(state.Content) == "" {
		return
	}
	payload := BuildSyntaxFlowFullChangeEvent(op, state.Path, state.Content, state.Version, state.SourceAction, state.ChangeReason)
	_, _ = loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_SYNTAXFLOW_RULE_CHANGE, loopSyntaxflowRuleChangeEventNode, payload)
}
