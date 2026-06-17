package loop_yaklangcode

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	yaklangEditorSyncPendingLoopKey  = "yaklang_editor_sync_pending"
	yaklangEditorSyncFlushingLoopKey = "yaklang_editor_sync_flushing"
	yaklangEditorSyncFlushedLoopKey  = "yaklang_editor_sync_flushed"
	yaklangCodeVersionLoopKey        = "yaklang_code_change_version"
	yaklangCodeSourceActionLoopKey   = "current_yaklang_code_source_action"
	yaklangCodeChangeReasonLoopKey   = "current_yaklang_code_change_reason"
	yaklangCodeChangeEventNode       = "yaklang_code_change"
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

// withYaklangDeferredEditorSync suppresses intermediate editor events during the loop and
// emits exactly one yaklang_code_change when the loop finishes.
func withYaklangDeferredEditorSync() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		reactloops.WithLoopEmitterProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
			if e == nil {
				return nil
			}
			if isYaklangEditorSyncFlushing(loop) {
				return e
			}
			if isYaklangEditorSyncFlushed(loop) {
				switch e.Type {
				case schema.EVENT_TYPE_YAKLANG_CODE_CHANGE, schema.EVENT_TYPE_YAKLANG_CODE_EDITOR:
					return nil
				}
			}
			switch e.Type {
			case schema.EVENT_TYPE_YAKLANG_CODE_CHANGE, schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME:
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
	if isYaklangEditorSyncFlushed(loop) {
		return
	}
	content := strings.TrimSpace(loop.Get("full_code"))
	if content == "" {
		return
	}
	if loop.GetInt(yaklangCodeVersionLoopKey) <= 0 && !isYaklangEditorSyncPending(loop) {
		return
	}

	path, eventOp, err := resolveYaklangDeliveryTarget(loop)
	if err != nil || path == "" {
		return
	}

	_ = writeYaklangDeliveryFile(path, content)
	loop.Set("filename", path)

	version := loop.GetInt(yaklangCodeVersionLoopKey)
	if version <= 0 {
		version = 1
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
	loop.Set(yaklangEditorSyncFlushedLoopKey, true)
}

func writeYaklangDeliveryFile(finalPath, content string) error {
	finalPath = strings.TrimSpace(finalPath)
	content = strings.TrimSpace(content)
	if finalPath == "" || content == "" {
		return nil
	}
	dir := filepath.Dir(finalPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(finalPath, []byte(content), 0o644)
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

func isYaklangEditorSyncFlushed(loop *reactloops.ReActLoop) bool {
	switch v := loop.GetVariable(yaklangEditorSyncFlushedLoopKey).(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
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
