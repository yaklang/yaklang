package loop_yaklangcode

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	yaklangEditorSyncPendingLoopKey  = "yaklang_editor_sync_pending"
	yaklangEditorSyncFlushingLoopKey = "yaklang_editor_sync_flushing"
	yaklangCodeVersionLoopKey        = "yaklang_code_change_version"
	yaklangCodeSourceActionLoopKey  = "current_yaklang_code_source_action"
	yaklangCodeChangeReasonLoopKey  = "current_yaklang_code_change_reason"
	yaklangCodeChangeEventNode      = "yaklang_code_change"
)

type yaklangCodeChangeEvent struct {
	Op           string                     `json:"op"`
	Code         yaklangCodeChangeEventCode `json:"code"`
	Reason       string                     `json:"reason,omitempty"`
	SourceAction string                     `json:"source_action,omitempty"`
}

type yaklangCodeChangeEventCode struct {
	Content string `json:"content"`
	Path    string `json:"path,omitempty"`
	Summary string `json:"summary,omitempty"`
	Version int    `json:"version"`
}

// withYaklangDeferredEditorSync keeps Yak Runner's left editor stable during multi-round
// write/modify iterations. Intermediate yaklang_code_change / yaklang_code_editor events
// are suppressed; one final yaklang_code_change is emitted when the loop finishes.
func withYaklangDeferredEditorSync() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		reactloops.WithLoopEmitterProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
			if e == nil {
				return nil
			}
			if isYaklangEditorSyncFlushing(loop) {
				return e
			}
			switch e.Type {
			case schema.EVENT_TYPE_YAKLANG_CODE_CHANGE, schema.EVENT_TYPE_YAKLANG_CODE_EDITOR:
				loop.Set(yaklangEditorSyncPendingLoopKey, true)
				return nil
			}
			return e
		})(loop)

		reactloops.WithOnPostIteraction(func(l *reactloops.ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, _ any, _ *reactloops.OnPostIterationOperator) {
			if !isDone {
				return
			}
			flushYaklangDeferredEditorSync(l)
		})(loop)
	}
}

func flushYaklangDeferredEditorSync(loop *reactloops.ReActLoop) {
	if loop == nil || loop.GetEmitter() == nil {
		return
	}
	if !isYaklangEditorSyncPending(loop) {
		return
	}

	content := strings.TrimSpace(loop.Get("full_code"))
	if content == "" {
		return
	}

	path := strings.TrimSpace(loop.Get("filename"))
	if isYaklangCodePreviewOnly(loop) {
		writtenPath, err := persistYaklangPreviewCode(loop, content)
		if err != nil {
			log.Errorf("preview mode: failed to persist generated code: %v", err)
			return
		}
		path = writtenPath
		_, _ = loop.GetEmitter().EmitPinFilename(path)
	}
	version := loop.GetInt(yaklangCodeVersionLoopKey)
	if version <= 0 {
		version = 1
	}

	eventOp := loopinfra.LoopYaklangCodeEventOpReplace
	if isYaklangCodePreviewOnly(loop) {
		eventOp = loopinfra.LoopYaklangCodeEventOpCreate
	}

	emitYaklangDeferredEditorSync(loop, yaklangCodeChangeEvent{
		Op: eventOp,
		Code: yaklangCodeChangeEventCode{
			Content: content,
			Path:    path,
			Summary: buildYaklangCodeSummary(content),
			Version: version,
		},
		Reason:       strings.TrimSpace(loop.Get(yaklangCodeChangeReasonLoopKey)),
		SourceAction: strings.TrimSpace(loop.Get(yaklangCodeSourceActionLoopKey)),
	})
	loop.Set(yaklangEditorSyncPendingLoopKey, false)
}

func emitYaklangDeferredEditorSync(loop *reactloops.ReActLoop, payload yaklangCodeChangeEvent) {
	loop.Set(yaklangEditorSyncFlushingLoopKey, true)
	defer loop.Set(yaklangEditorSyncFlushingLoopKey, false)
	_, _ = loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_CHANGE, yaklangCodeChangeEventNode, payload)
}

func isYaklangEditorSyncFlushing(loop *reactloops.ReActLoop) bool {
	switch v := loop.GetVariable(yaklangEditorSyncFlushingLoopKey).(type) {
	case bool:
		return v
	default:
		return false
	}
}

func isYaklangEditorSyncPending(loop *reactloops.ReActLoop) bool {
	switch v := loop.GetVariable(yaklangEditorSyncPendingLoopKey).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return false
	}
}

func buildYaklangCodeSummary(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if len(content) > 200 {
		return content[:200] + "..."
	}
	return content
}
