package loop_syntaxflow_rule

import "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"

const defaultSyntaxFlowRuleChangeSource = "syntaxflow_rule"

func BuildSyntaxFlowFullChangeEvent(op, path, content string, version int, sourceAction, reason string) loopinfra.CodeChangeEvent {
	return loopinfra.BuildCodeFullChangeEvent(op, path, content, version, sourceAction, reason, defaultSyntaxFlowRuleChangeSource)
}

func BuildSyntaxFlowPatchChangeEvent(path string, patch *loopinfra.CodeDeliveryPatch, version int, sourceAction, reason string) loopinfra.CodeChangeEvent {
	return loopinfra.BuildCodePatchChangeEvent(path, patch, version, sourceAction, reason, defaultSyntaxFlowRuleChangeSource)
}
