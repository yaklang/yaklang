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

// YaklangCodePatchMeta describes a single frontend-applicable code fragment.
// code.content carries PatchFragment; metadata lives in code.patch.
type YaklangCodePatchMeta struct {
	Kind       string `json:"kind"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	InsertLine int    `json:"insert_line,omitempty"`
	OldSnippet string `json:"old_snippet,omitempty"`
}

// YaklangCodeDeliveryPatch is stored on the loop when a yaklang file action commits.
type YaklangCodeDeliveryPatch struct {
	Fragment   string
	Meta       YaklangCodePatchMeta
	LineBase   int
	SourceAction string
	ChangeReason string
	Version    int
}

// YaklangCodeChangeEventCode is the wire JSON shape for yaklang_code_change.code.
type YaklangCodeChangeEventCode struct {
	Content  string                `json:"content"`
	Path     string                `json:"path,omitempty"`
	Summary  string                `json:"summary,omitempty"`
	Version  int                   `json:"version"`
	ChangeID string                `json:"change_id,omitempty"`
	LineBase int                   `json:"line_base,omitempty"`
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
		LineBase: lineBase,
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindLineRange,
			StartLine:  YaklangAbsoluteLine(startLine, lineBase),
			EndLine:    YaklangAbsoluteLine(endLine, lineBase),
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchSnippet(fragment, oldSnippet string, lineBase int) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		LineBase: lineBase,
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindSnippet,
			OldSnippet: oldSnippet,
		},
	}
}

func BuildYaklangPatchInsert(fragment string, insertLine, lineBase int) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		Fragment: strings.TrimSpace(fragment),
		LineBase: lineBase,
		Meta: YaklangCodePatchMeta{
			Kind:       YaklangPatchKindInsert,
			InsertLine: YaklangAbsoluteLine(insertLine, lineBase),
		},
	}
}

func BuildYaklangPatchDelete(startLine, endLine int, oldSnippet string, lineBase int) *YaklangCodeDeliveryPatch {
	return &YaklangCodeDeliveryPatch{
		LineBase: lineBase,
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
		version = patch.Version
	}
	if version <= 0 {
		version = 1
	}
	sourceAction = firstNonEmptyYaklangString(sourceAction, patch.SourceAction)
	reason = firstNonEmptyYaklangString(reason, patch.ChangeReason)
	meta := patch.Meta
	return YaklangCodeChangeEvent{
		Op: LoopYaklangCodeEventOpPatch,
		Code: YaklangCodeChangeEventCode{
			Content:  patch.Fragment,
			Path:     strings.TrimSpace(path),
			Summary:  buildLoopYaklangCodeSummary(patch.Fragment),
			Version:  version,
			ChangeID: BuildYaklangCodeChangeID(sourceAction, version),
			LineBase: patch.LineBase,
			Patch:    &meta,
		},
		Reason:       reason,
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
