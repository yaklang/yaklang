package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	CodeEventOpPatch = "patch"

	CodePatchKindLineRange = "line_range"
	CodePatchKindSnippet   = "snippet"
	CodePatchKindInsert    = "insert"
	CodePatchKindDelete    = "delete"
	CodePatchKindFull      = "full"

	codeLastDeliveryPatchLoopKey = "code_last_delivery_patch"
	codeChangeIDFallbackSource   = "code"
)

// CodePatchMeta describes how to apply code.content (fragment) on the frontend.
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

// CodeChangeEventCode is the shared wire JSON shape for editor change events.
type CodeChangeEventCode struct {
	Content  string         `json:"content"`
	Path     string         `json:"path,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Version  int            `json:"version"`
	ChangeID string         `json:"change_id,omitempty"`
	Patch    *CodePatchMeta `json:"patch,omitempty"`
}

// CodeChangeEvent is the shared wire JSON payload for editor change events.
type CodeChangeEvent struct {
	Op           string              `json:"op"`
	Code         CodeChangeEventCode `json:"code"`
	Reason       string              `json:"reason,omitempty"`
	SourceAction string              `json:"source_action,omitempty"`
}

func CodeAbsoluteLine(relativeLine, lineBase int) int {
	if relativeLine <= 0 {
		return relativeLine
	}
	if lineBase <= 0 {
		return relativeLine
	}
	return relativeLine + lineBase
}

func BuildCodePatchLineRange(fragment string, startLine, endLine int, oldSnippet string, lineBase int) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind:       CodePatchKindLineRange,
			StartLine:  CodeAbsoluteLine(startLine, lineBase),
			EndLine:    CodeAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildCodePatchSnippet(fragment, oldSnippet string, lineBase int) *CodeDeliveryPatch {
	_ = lineBase // reserved for callers that pass seed offset; snippet match is text-based
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind:       CodePatchKindSnippet,
			OldSnippet: oldSnippet,
		},
	}
}

func BuildCodePatchInsert(fragment string, insertLine, lineBase int) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind:       CodePatchKindInsert,
			InsertLine: CodeAbsoluteLine(insertLine, lineBase),
		},
	}
}

func BuildCodePatchDelete(startLine, endLine int, oldSnippet string, lineBase int) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Meta: CodePatchMeta{
			Kind:       CodePatchKindDelete,
			StartLine:  CodeAbsoluteLine(startLine, lineBase),
			EndLine:    CodeAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildCodePatchFull(fragment string) *CodeDeliveryPatch {
	return &CodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: CodePatchMeta{
			Kind: CodePatchKindFull,
		},
	}
}

func SetLoopCodeDeliveryPatch(loop *reactloops.ReActLoop, patch *CodeDeliveryPatch) {
	if loop == nil || patch == nil {
		return
	}
	loop.Set(codeLastDeliveryPatchLoopKey, patch)
}

func GetLoopCodeDeliveryPatch(loop *reactloops.ReActLoop) *CodeDeliveryPatch {
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

func ClearLoopCodeDeliveryPatch(loop *reactloops.ReActLoop) {
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
		sourceAction = codeChangeIDFallbackSource
	}
	if version <= 0 {
		version = 1
	}
	return fmt.Sprintf("%s:%d", sourceAction, version)
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
		Op: CodeEventOpPatch,
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
		op = CodeEventOpReplace
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
