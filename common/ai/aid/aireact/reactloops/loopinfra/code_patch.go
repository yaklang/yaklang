package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	LoopYaklangCodeEventOpPatch = "patch"

	CodePatchKindLineRange = "line_range"
	CodePatchKindSnippet   = "snippet"
	CodePatchKindInsert    = "insert"
	CodePatchKindDelete    = "delete"
	CodePatchKindFull      = "full"

	YaklangPatchKindLineRange = CodePatchKindLineRange
	YaklangPatchKindSnippet   = CodePatchKindSnippet
	YaklangPatchKindInsert    = CodePatchKindInsert
	YaklangPatchKindDelete    = CodePatchKindDelete
	YaklangPatchKindFull      = CodePatchKindFull

	SyntaxFlowPatchKindLineRange = CodePatchKindLineRange
	SyntaxFlowPatchKindSnippet   = CodePatchKindSnippet
	SyntaxFlowPatchKindInsert    = CodePatchKindInsert
	SyntaxFlowPatchKindDelete    = CodePatchKindDelete
	SyntaxFlowPatchKindFull      = CodePatchKindFull

	codeLastDeliveryPatchLoopKey = "yaklang_last_delivery_patch"

	defaultYaklangCodeChangeSource    = "yaklang_code"
	defaultSyntaxFlowRuleChangeSource = "syntaxflow_rule"
)

// CodePatchMeta describes how to apply code.content (fragment) on the frontend.
// Shared by yaklang_code_change and syntaxflow_rule_change wire payloads.
type CodePatchMeta struct {
	Kind       string `json:"kind"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	InsertLine int    `json:"insert_line,omitempty"`
	OldSnippet string `json:"old_snippet,omitempty"`
}

// CodeDeliveryPatch is stored on the loop when a single-file action commits.
// Line numbers in Meta are already absolute (1-based file lines).
type CodeDeliveryPatch struct {
	Fragment string
	Meta     CodePatchMeta
}

// CodeChangeEventCode is the shared wire JSON shape for *.code on editor change events.
type CodeChangeEventCode struct {
	Content  string         `json:"content"`
	Path     string         `json:"path,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Version  int            `json:"version"`
	ChangeID string         `json:"change_id,omitempty"`
	Patch    *CodePatchMeta `json:"patch,omitempty"`
}

// CodeChangeEvent is the shared wire JSON payload for yaklang_code_change /
// syntaxflow_rule_change (frontends can share normalize/review helpers).
type CodeChangeEvent struct {
	Op           string              `json:"op"`
	Code         CodeChangeEventCode `json:"code"`
	Reason       string              `json:"reason,omitempty"`
	SourceAction string              `json:"source_action,omitempty"`
}

// Compatibility aliases — prefer Code* names for new code.
type (
	YaklangCodePatchMeta       = CodePatchMeta
	YaklangCodeDeliveryPatch   = CodeDeliveryPatch
	YaklangCodeChangeEventCode = CodeChangeEventCode
	YaklangCodeChangeEvent     = CodeChangeEvent

	SyntaxFlowRulePatchMeta       = CodePatchMeta
	SyntaxFlowRuleChangeEventCode = CodeChangeEventCode
	SyntaxFlowRuleChangeEvent     = CodeChangeEvent
)

func YaklangAbsoluteLine(relativeLine, lineBase int) int {
	if relativeLine <= 0 {
		return relativeLine
	}
	if lineBase <= 0 {
		return relativeLine
	}
	return relativeLine + lineBase
}

func BuildYaklangPatchLineRange(fragment string, startLine, endLine int, oldSnippet string, lineBase int) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind:       CodePatchKindLineRange,
			StartLine:  YaklangAbsoluteLine(startLine, lineBase),
			EndLine:    YaklangAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchSnippet(fragment, oldSnippet string, lineBase int) *CodeDeliveryPatch {
	_ = lineBase // reserved for callers that pass seed offset; snippet match is text-based
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind:       CodePatchKindSnippet,
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchInsert(fragment string, insertLine, lineBase int) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind:       CodePatchKindInsert,
			InsertLine: YaklangAbsoluteLine(insertLine, lineBase),
		},
	}
}

func BuildYaklangPatchDelete(startLine, endLine int, oldSnippet string, lineBase int) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Meta: CodePatchMeta{
			Kind:       CodePatchKindDelete,
			StartLine:  YaklangAbsoluteLine(startLine, lineBase),
			EndLine:    YaklangAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchFull(fragment string) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind: CodePatchKindFull,
		},
	}
}

func SetLoopYaklangDeliveryPatch(loop *reactloops.ReActLoop, patch *CodeDeliveryPatch) {
	if loop == nil || patch == nil {
		return
	}
	loop.Set(codeLastDeliveryPatchLoopKey, patch)
}

func GetLoopYaklangDeliveryPatch(loop *reactloops.ReActLoop) *CodeDeliveryPatch {
	if loop == nil {
		return nil
	}
	switch v := loop.GetVariable(codeLastDeliveryPatchLoopKey).(type) {
	case *CodeDeliveryPatch:
		return v
	case CodeDeliveryPatch:
		p := v
		return &p
	default:
		return nil
	}
}

func ClearLoopYaklangDeliveryPatch(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Delete(codeLastDeliveryPatchLoopKey)
}

func BuildCodeChangeID(sourceAction, defaultSource string, version int) string {
	sourceAction = strings.TrimSpace(sourceAction)
	if sourceAction == "" {
		sourceAction = strings.TrimSpace(defaultSource)
	}
	if sourceAction == "" {
		sourceAction = defaultYaklangCodeChangeSource
	}
	if version <= 0 {
		version = 1
	}
	return fmt.Sprintf("%s:%d", sourceAction, version)
}

func BuildYaklangCodeChangeID(sourceAction string, version int) string {
	return BuildCodeChangeID(sourceAction, defaultYaklangCodeChangeSource, version)
}

func BuildSyntaxFlowRuleChangeID(sourceAction string, version int) string {
	return BuildCodeChangeID(sourceAction, defaultSyntaxFlowRuleChangeSource, version)
}

func BuildCodePatchChangeEvent(path string, patch *CodeDeliveryPatch, version int, sourceAction, reason, defaultSource string) CodeChangeEvent {
	if patch == nil {
		return CodeChangeEvent{}
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = strings.TrimSpace(sourceAction)
	meta := patch.Meta
	return CodeChangeEvent{
		Op: LoopYaklangCodeEventOpPatch,
		Code: CodeChangeEventCode{
			Content:  patch.Fragment,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(patch.Fragment),
			Version:  version,
			ChangeID: BuildCodeChangeID(sourceAction, defaultSource, version),
			Patch:    &meta,
		},
		Reason:       strings.TrimSpace(reason),
		SourceAction: sourceAction,
	}
}

func BuildCodeFullChangeEvent(op, path, content string, version int, sourceAction, reason, defaultSource string) CodeChangeEvent {
	content = strings.TrimSpace(content)
	if strings.TrimSpace(op) == "" {
		op = LoopYaklangCodeEventOpReplace
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = strings.TrimSpace(sourceAction)
	return CodeChangeEvent{
		Op: op,
		Code: CodeChangeEventCode{
			Content:  content,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(content),
			Version:  version,
			ChangeID: BuildCodeChangeID(sourceAction, defaultSource, version),
		},
		Reason:       strings.TrimSpace(reason),
		SourceAction: sourceAction,
	}
}

func BuildYaklangPatchChangeEvent(path string, patch *CodeDeliveryPatch, version int, sourceAction, reason string) CodeChangeEvent {
	return BuildCodePatchChangeEvent(path, patch, version, sourceAction, reason, defaultYaklangCodeChangeSource)
}

func BuildYaklangFullChangeEvent(op, path, content string, version int, sourceAction, reason string) CodeChangeEvent {
	return BuildCodeFullChangeEvent(op, path, content, version, sourceAction, reason, defaultYaklangCodeChangeSource)
}

// BuildSyntaxFlowFullChangeEvent builds a full-content syntaxflow_rule_change payload.
func BuildSyntaxFlowFullChangeEvent(op, path, content string, version int, sourceAction, reason string) CodeChangeEvent {
	return BuildCodeFullChangeEvent(op, path, content, version, sourceAction, reason, defaultSyntaxFlowRuleChangeSource)
}

// BuildSyntaxFlowPatchChangeEvent builds an op=patch syntaxflow_rule_change payload from a delivery patch.
func BuildSyntaxFlowPatchChangeEvent(path string, patch *CodeDeliveryPatch, version int, sourceAction, reason string) CodeChangeEvent {
	return BuildCodePatchChangeEvent(path, patch, version, sourceAction, reason, defaultSyntaxFlowRuleChangeSource)
}
