package loop_yaklangcode

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	yaklangEditorSyncPendingLoopKey    = "yaklang_editor_sync_pending"
	yaklangEditorSyncFlushingLoopKey   = "yaklang_editor_sync_flushing"
	yaklangEditorLastEmittedContentKey = "yaklang_editor_last_emitted_content"
	yaklangEditorDeliveryPathLoopKey   = "yaklang_editor_delivery_path"
	yaklangEditorDeliveryOpLoopKey     = "yaklang_editor_delivery_op"
	yaklangCodeVersionLoopKey          = "yaklang_code_change_version"
	yaklangCodeSourceActionLoopKey     = "current_yaklang_code_source_action"
	yaklangCodeChangeReasonLoopKey     = "current_yaklang_code_change_reason"
	yaklangCodeChangeEventNode         = "yaklang_code_change"
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

// withYaklangDeferredEditorSync installs the editor-sync event processor for the yaklang code loop.
//
// Delivery policy (关键: 第一次输出代码就覆盖编辑器文件):
//   - Replace targets (an open editor file or a concrete file path): every code change is delivered
//     to the frontend LIVE, i.e. the very first write_code/modify_code immediately overwrites the
//     editor file, and later edits keep it in sync. This is what "直接开始输出第一次代码就覆盖文件" needs.
//   - Create targets (a brand-new gen_code_*.yak, no open editor file): the new-file path is allocated
//     once by the frontend, so intermediate events are suppressed and exactly one create event is
//     delivered when the loop finishes.
//
// The internal yaklang_code_editor / filesystem_pin_filename events always carry aispace staging
// paths and are suppressed; only the resolved yaklang_code_change reaches the frontend.
func withYaklangDeferredEditorSync() reactloops.ReActLoopOption {
	return func(loop *reactloops.ReActLoop) {
		reactloops.WithLoopEmitterProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
			if e == nil {
				return nil
			}
			// Our own resolved yaklang_code_change is re-emitted with this flag set; let it pass.
			if isYaklangEditorSyncFlushing(loop) {
				return e
			}
			switch e.Type {
			case schema.EVENT_TYPE_YAKLANG_CODE_CHANGE:
				// Suppress the internal event (staging path) and deliver a resolved one instead.
				liveSyncYaklangEditorOnChange(loop)
				return nil
			case schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME:
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

		// Safety net: some terminal paths break out of the iteration loop without isDone post hooks.
		reactloops.WithOnLoopRelease(func() {
			flushYaklangDeferredEditorSync(loop)
		})(loop)
	}
}

// liveSyncYaklangEditorOnChange runs inside the emitter processor on every internal
// yaklang_code_change. Replace targets are delivered immediately (first code output overwrites the
// editor file); create targets are deferred until the loop finishes.
func liveSyncYaklangEditorOnChange(loop *reactloops.ReActLoop) {
	if loop == nil || loop.GetEmitter() == nil {
		return
	}
	content := strings.TrimSpace(loop.Get("full_code"))
	if content == "" {
		return
	}

	path, eventOp, err := resolveCachedYaklangDeliveryTarget(loop)
	if err != nil {
		log.Warnf("live editor sync: resolve delivery target failed: %v", err)
		return
	}
	if path == "" {
		return
	}

	// New-file delivery is finalized once when the loop ends; the frontend allocates the
	// destination path a single time, so emitting multiple create events would fork files.
	if eventOp == loopinfra.LoopYaklangCodeEventOpCreate {
		loop.Set(yaklangEditorSyncPendingLoopKey, true)
		return
	}

	emitYaklangEditorDelivery(loop, path, eventOp, content)
}

// flushYaklangDeferredEditorSync delivers the final yaklang_code_change when the loop finishes.
// For replace targets this is usually a no-op (already delivered live, deduplicated by content);
// for create targets this emits the single new-file event.
func flushYaklangDeferredEditorSync(loop *reactloops.ReActLoop) {
	if loop == nil || loop.GetEmitter() == nil {
		return
	}
	content := strings.TrimSpace(loop.Get("full_code"))
	if content == "" {
		log.Infof("skip yaklang_code_change flush: full_code is empty")
		return
	}
	// Already delivered this exact content (e.g. live edit sync); nothing to do.
	if content == loop.Get(yaklangEditorLastEmittedContentKey) {
		loop.Set(yaklangEditorSyncPendingLoopKey, false)
		return
	}

	committed := loopinfra.HasCommittedYaklangCodeChange(loop, "full_code")
	pending := isYaklangEditorSyncPending(loop)
	if !committed && !pending {
		return
	}
	if !committed && loopinfra.IsLoopCodeSeededOnly(loop) && yaklangDeferredFlushWouldRepeatSeed(loop, content) {
		log.Infof("skip yaklang_code_change flush: no committed edits in this loop (seed=%d bytes)", len(strings.TrimSpace(loop.Get(loopinfra.LoopVarInitSeedFullCode))))
		loop.Set(yaklangEditorSyncPendingLoopKey, false)
		return
	}

	path, eventOp, err := resolveCachedYaklangDeliveryTarget(loop)
	if err != nil {
		log.Warnf("skip yaklang_code_change flush: resolve delivery target failed: %v", err)
		return
	}
	if path == "" {
		log.Warnf("skip yaklang_code_change flush: delivery path is empty")
		return
	}

	emitYaklangEditorDelivery(loop, path, eventOp, content)
}

// resolveCachedYaklangDeliveryTarget resolves the frontend delivery path + op once and caches it,
// so live edits and the final flush all target a single stable file.
func resolveCachedYaklangDeliveryTarget(loop *reactloops.ReActLoop) (string, string, error) {
	if cachedPath := strings.TrimSpace(loop.Get(yaklangEditorDeliveryPathLoopKey)); cachedPath != "" {
		cachedOp := strings.TrimSpace(loop.Get(yaklangEditorDeliveryOpLoopKey))
		if cachedOp == "" {
			cachedOp = loopinfra.LoopYaklangCodeEventOpReplace
		}
		return cachedPath, cachedOp, nil
	}

	path, eventOp, err := resolveYaklangDeliveryTarget(loop)
	if err != nil {
		return "", "", err
	}
	if path == "" {
		return "", "", nil
	}
	loop.Set(yaklangEditorDeliveryPathLoopKey, path)
	loop.Set(yaklangEditorDeliveryOpLoopKey, eventOp)
	return path, eventOp, nil
}

// emitYaklangEditorDelivery writes the delivery file and emits one resolved yaklang_code_change.
// It deduplicates by last-emitted content so repeated flushes / identical edits stay quiet.
func emitYaklangEditorDelivery(loop *reactloops.ReActLoop, path, eventOp, content string) {
	if loop == nil || loop.GetEmitter() == nil {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" || strings.TrimSpace(path) == "" {
		return
	}
	if content == loop.Get(yaklangEditorLastEmittedContentKey) {
		loop.Set(yaklangEditorSyncPendingLoopKey, false)
		return
	}

	if writeErr := writeYaklangDeliveryFile(path, content); writeErr != nil {
		log.Warnf("yaklang_code_change: write delivery file failed: %v", writeErr)
		return
	}

	version := loopinfra.ResolvedYaklangCodeChangeVersion(loop, "full_code")
	if version <= 0 {
		version = 1
	}
	if strings.TrimSpace(eventOp) == "" {
		eventOp = loopinfra.LoopYaklangCodeEventOpReplace
	}

	emitYaklangCodeChangeEvent(loop, yaklangCodeChangeEvent{
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
	log.Infof("yaklang_code_change delivered: path=%s op=%s version=%d bytes=%d", path, eventOp, version, len(content))
	loop.Set(yaklangEditorLastEmittedContentKey, content)
	loop.Set(yaklangEditorSyncPendingLoopKey, false)
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

// emitYaklangCodeChangeEvent re-emits a resolved yaklang_code_change. The flushing flag makes the
// loop emitter processor pass this event straight through instead of re-intercepting it.
func emitYaklangCodeChangeEvent(loop *reactloops.ReActLoop, payload yaklangCodeChangeEvent) {
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

func yaklangDeferredFlushWouldRepeatSeed(loop *reactloops.ReActLoop, content string) bool {
	if loop == nil {
		return false
	}
	seed := strings.TrimSpace(loop.Get(loopinfra.LoopVarInitSeedFullCode))
	if seed == "" {
		return false
	}
	return strings.TrimSpace(content) == seed
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
