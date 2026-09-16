package loopinfra

import (
	"fmt"
	"strings"
)

const (
	SyntaxFlowPatchKindLineRange = YaklangPatchKindLineRange
	SyntaxFlowPatchKindSnippet   = YaklangPatchKindSnippet
	SyntaxFlowPatchKindInsert    = YaklangPatchKindInsert
	SyntaxFlowPatchKindDelete    = YaklangPatchKindDelete
	SyntaxFlowPatchKindFull      = YaklangPatchKindFull
)

// SyntaxFlowRulePatchMeta describes how to apply rule.code.content (fragment) on the frontend.
type SyntaxFlowRulePatchMeta struct {
	Kind       string `json:"kind"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	InsertLine int    `json:"insert_line,omitempty"`
	OldSnippet string `json:"old_snippet,omitempty"`
}

// SyntaxFlowRuleChangeEventCode is the wire JSON shape for syntaxflow_rule_change.code.
type SyntaxFlowRuleChangeEventCode struct {
	Content  string                   `json:"content"`
	Path     string                   `json:"path,omitempty"`
	Summary  string                   `json:"summary,omitempty"`
	Version  int                      `json:"version"`
	ChangeID string                   `json:"change_id,omitempty"`
	Patch    *SyntaxFlowRulePatchMeta `json:"patch,omitempty"`
}

// SyntaxFlowRuleChangeEvent is the wire JSON payload for syntaxflow_rule_change.
// Field layout mirrors yaklang_code_change so frontends can share normalize/review helpers.
type SyntaxFlowRuleChangeEvent struct {
	Op           string                        `json:"op"`
	Code         SyntaxFlowRuleChangeEventCode `json:"code"`
	Reason       string                        `json:"reason,omitempty"`
	SourceAction string                        `json:"source_action,omitempty"`
}

func BuildSyntaxFlowRuleChangeID(sourceAction string, version int) string {
	sourceAction = strings.TrimSpace(sourceAction)
	if sourceAction == "" {
		sourceAction = "syntaxflow_rule"
	}
	if version <= 0 {
		version = 1
	}
	return fmt.Sprintf("%s:%d", sourceAction, version)
}

// BuildSyntaxFlowFullChangeEvent builds a full-content syntaxflow_rule_change payload.
func BuildSyntaxFlowFullChangeEvent(op, path, content string, version int, sourceAction, reason string) SyntaxFlowRuleChangeEvent {
	content = strings.TrimSpace(content)
	if strings.TrimSpace(op) == "" {
		op = LoopYaklangCodeEventOpReplace
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = strings.TrimSpace(sourceAction)
	return SyntaxFlowRuleChangeEvent{
		Op: op,
		Code: SyntaxFlowRuleChangeEventCode{
			Content:  content,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(content),
			Version:  version,
			ChangeID: BuildSyntaxFlowRuleChangeID(sourceAction, version),
		},
		Reason:       strings.TrimSpace(reason),
		SourceAction: sourceAction,
	}
}

// BuildSyntaxFlowPatchChangeEvent builds an op=patch syntaxflow_rule_change payload from a delivery patch.
func BuildSyntaxFlowPatchChangeEvent(path string, patch *YaklangCodeDeliveryPatch, version int, sourceAction, reason string) SyntaxFlowRuleChangeEvent {
	if patch == nil {
		return SyntaxFlowRuleChangeEvent{}
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = strings.TrimSpace(sourceAction)
	meta := SyntaxFlowRulePatchMeta{
		Kind:       patch.Meta.Kind,
		StartLine:  patch.Meta.StartLine,
		EndLine:    patch.Meta.EndLine,
		InsertLine: patch.Meta.InsertLine,
		OldSnippet: patch.Meta.OldSnippet,
	}
	return SyntaxFlowRuleChangeEvent{
		Op: LoopYaklangCodeEventOpPatch,
		Code: SyntaxFlowRuleChangeEventCode{
			Content:  patch.Fragment,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(patch.Fragment),
			Version:  version,
			ChangeID: BuildSyntaxFlowRuleChangeID(sourceAction, version),
			Patch:    &meta,
		},
		Reason:       strings.TrimSpace(reason),
		SourceAction: sourceAction,
	}
}
