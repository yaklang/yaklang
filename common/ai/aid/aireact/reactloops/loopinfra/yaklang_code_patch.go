package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	LoopYaklangCodeEventOpPatch = "patch"

	YaklangPatchKindLineRange = "line_range"
	YaklangPatchKindSnippet   = "snippet"
	YaklangPatchKindInsert    = "insert"
	YaklangPatchKindDelete    = "delete"
	YaklangPatchKindFull      = "full"

	yaklangLastDeliveryPatchLoopKey = "yaklang_last_delivery_patch"
)

// YaklangCodePatchMeta describes how to apply code.content (fragment) on the frontend.
type YaklangCodePatchMeta struct {
	Kind       string `json:"kind"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	InsertLine int    `json:"insert_line,omitempty"`
	OldSnippet string `json:"old_snippet,omitempty"`
}

// YaklangCodeDeliveryPatch is stored on the loop when a yaklang file action commits.
// Line numbers in Meta are already absolute (1-based file lines).
type YaklangCodeDeliveryPatch struct {
	Fragment string
	Meta     YaklangCodePatchMeta
}

// YaklangCodeChangeEventCode is the wire JSON shape for yaklang_code_change.code.
type YaklangCodeChangeEventCode struct {
	Content  string                `json:"content"`
	Path     string                `json:"path,omitempty"`
	Summary  string                `json:"summary,omitempty"`
	Version  int                   `json:"version"`
	ChangeID string                `json:"change_id,omitempty"`
	Patch    *YaklangCodePatchMeta `json:"patch,omitempty"`
}

// YaklangCodeChangeEvent is the wire JSON payload for yaklang_code_change.
type YaklangCodeChangeEvent struct {
	Op           string                     `json:"op"`
	Code         YaklangCodeChangeEventCode `json:"code"`
	Reason       string                     `json:"reason,omitempty"`
	SourceAction string                     `json:"source_action,omitempty"`
}

func YaklangAbsoluteLine(relativeLine, lineBase int) int {
	if relativeLine <= 0 {
		return relativeLine
	}
	if lineBase <= 0 {
		return relativeLine
	}
	return relativeLine + lineBase
}

func BuildYaklangPatchLineRange(fragment string, startLine, endLine int, oldSnippet string, lineBase int) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindLineRange,
			StartLine:  YaklangAbsoluteLine(startLine, lineBase),
			EndLine:    YaklangAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchSnippet(fragment, oldSnippet string, lineBase int) *YaklangCodeDeliveryPatch {
	_ = lineBase // reserved for callers that pass seed offset; snippet match is text-based
	return &YaklangCodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindSnippet,
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchInsert(fragment string, insertLine, lineBase int) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindInsert,
			InsertLine: YaklangAbsoluteLine(insertLine, lineBase),
		},
	}
}

func BuildYaklangPatchDelete(startLine, endLine int, oldSnippet string, lineBase int) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindDelete,
			StartLine:  YaklangAbsoluteLine(startLine, lineBase),
			EndLine:    YaklangAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchFull(fragment string) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		Meta: YaklangCodePatchMeta{
			Kind: YaklangPatchKindFull,
		},
	}
}

func SetLoopYaklangDeliveryPatch(loop *reactloops.ReActLoop, patch *YaklangCodeDeliveryPatch) {
	if loop == nil || patch == nil {
		return
	}
	loop.Set(yaklangLastDeliveryPatchLoopKey, patch)
}

func GetLoopYaklangDeliveryPatch(loop *reactloops.ReActLoop) *YaklangCodeDeliveryPatch {
	if loop == nil {
		return nil
	}
	switch v := loop.GetVariable(yaklangLastDeliveryPatchLoopKey).(type) {
	case *YaklangCodeDeliveryPatch:
		return v
	case YaklangCodeDeliveryPatch:
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
	loop.Delete(yaklangLastDeliveryPatchLoopKey)
}

func BuildYaklangCodeChangeID(sourceAction string, version int) string {
	sourceAction = strings.TrimSpace(sourceAction)
	if sourceAction == "" {
		sourceAction = "yaklang_code"
	}
	if version <= 0 {
		version = 1
	}
	return fmt.Sprintf("%s:%d", sourceAction, version)
}

func BuildYaklangPatchChangeEvent(path string, patch *YaklangCodeDeliveryPatch, version int, sourceAction, reason string) YaklangCodeChangeEvent {
	if patch == nil {
		return YaklangCodeChangeEvent{}
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = strings.TrimSpace(sourceAction)
	meta := patch.Meta
	return YaklangCodeChangeEvent{
		Op: LoopYaklangCodeEventOpPatch,
		Code: YaklangCodeChangeEventCode{
			Content:  patch.Fragment,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(patch.Fragment),
			Version:  version,
			ChangeID: BuildYaklangCodeChangeID(sourceAction, version),
			Patch:    &meta,
		},
		Reason:       strings.TrimSpace(reason),
		SourceAction: sourceAction,
	}
}

func BuildYaklangFullChangeEvent(op, path, content string, version int, sourceAction, reason string) YaklangCodeChangeEvent {
	content = strings.TrimSpace(content)
	if strings.TrimSpace(op) == "" {
		op = LoopYaklangCodeEventOpReplace
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = strings.TrimSpace(sourceAction)
	return YaklangCodeChangeEvent{
		Op: op,
		Code: YaklangCodeChangeEventCode{
			Content:  content,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(content),
			Version:  version,
			ChangeID: BuildYaklangCodeChangeID(sourceAction, version),
		},
		Reason:       strings.TrimSpace(reason),
		SourceAction: sourceAction,
	}
}
