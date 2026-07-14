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
	yaklangEditorLastEmittedVersionKey = "yaklang_editor_last_emitted_version"
	yaklangEditorDeliveryPathLoopKey   = "yaklang_editor_delivery_path"
	yaklangEditorDeliveryOpLoopKey     = "yaklang_editor_delivery_op"
	yaklangCodeSourceActionLoopKey     = "current_yaklang_code_source_action"
	yaklangCodeChangeReasonLoopKey     = "current_yaklang_code_change_reason"
	yaklangCodeChangeEventNode         = "yaklang_code_change"
)

// withYaklangDeferredEditorSync installs the editor-sync event processor for the yaklang code loop.
//
// Delivery policy:
//   - Replace targets (open editor file): each committed edit emits op=patch with a code fragment in
//     code.content plus code.patch metadata (line_range / snippet / insert / delete / full).
//     The delivery file on disk is still updated with the loop's full_code after each edit.
//   - Create targets (gen_code_*.yak): intermediate patch events are suppressed; exactly one
//     op=create with full code.content is delivered when the loop finishes.
//   - Loop flush for replace targets emits op=replace with full code.content so the frontend can
//     run a final review diff against the task-start baseline.
//
// Internal yaklang_code_editor / filesystem_pin_filename events are suppressed.
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
			case schema.EVENT_TYPE_YAKLANG_CODE_CHANGE:
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

		reactloops.WithOnLoopRelease(func() {
			flushYaklangDeferredEditorSync(loop)
		})(loop)
	}
}

func liveSyncYaklangEditorOnChange(loop *reactloops.ReActLoop) {
	if loop == nil || loop.GetEmitter() == nil {
		return
	}
	fullCode := strings.TrimSpace(loop.Get("full_code"))
	if fullCode == "" {
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

	if eventOp == loopinfra.LoopYaklangCodeEventOpCreate {
		loop.Set(yaklangEditorSyncPendingLoopKey, true)
		return
	}

	if writeErr := writeYaklangDeliveryFile(path, fullCode); writeErr != nil {
		log.Warnf("yaklang_code_change: write delivery file failed: %v", writeErr)
		return
	}

	deliveryPatch := loopinfra.GetLoopYaklangDeliveryPatch(loop)
	if deliveryPatch != nil {
		emitYaklangEditorPatchDelivery(loop, path, fullCode, deliveryPatch)
		return
	}

	emitYaklangEditorFullDelivery(loop, path, loopinfra.LoopYaklangCodeEventOpReplace, fullCode)
}

func flushYaklangDeferredEditorSync(loop *reactloops.ReActLoop) {
	if loop == nil || loop.GetEmitter() == nil {
		return
	}
	content := strings.TrimSpace(loop.Get("full_code"))
	if content == "" {
		log.Infof("skip yaklang_code_change flush: full_code is empty")
		return
	}
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

	emitYaklangEditorFullDelivery(loop, path, eventOp, content)
}

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

func emitYaklangEditorPatchDelivery(loop *reactloops.ReActLoop, path, fullCode string, patch *loopinfra.YaklangCodeDeliveryPatch) {
	if loop == nil || patch == nil {
		return
	}
	version := loopinfra.ResolvedYaklangCodeChangeVersion(loop, "full_code")
	if version <= 0 {
		version = 1
	}
	if version <= loop.GetInt(yaklangEditorLastEmittedVersionKey) {
		return
	}

	sourceAction := strings.TrimSpace(loop.Get(yaklangCodeSourceActionLoopKey))
	reason := strings.TrimSpace(loop.Get(yaklangCodeChangeReasonLoopKey))
	payload := loopinfra.BuildYaklangPatchChangeEvent(path, patch, version, sourceAction, reason)

	emitYaklangCodeChangeEvent(loop, payload)
	log.Infof(
		"yaklang_code_change patch delivered: path=%s kind=%s version=%d bytes=%d",
		path, patch.Meta.Kind, version, len(payload.Code.Content),
	)
	loop.Set(yaklangEditorLastEmittedVersionKey, version)
	loop.Set(yaklangEditorLastEmittedContentKey, strings.TrimSpace(fullCode))
	loop.Set(yaklangEditorSyncPendingLoopKey, false)
	loopinfra.ClearLoopYaklangDeliveryPatch(loop)
}

func emitYaklangEditorFullDelivery(loop *reactloops.ReActLoop, path, eventOp, content string) {
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

	sourceAction := strings.TrimSpace(loop.Get(yaklangCodeSourceActionLoopKey))
	reason := strings.TrimSpace(loop.Get(yaklangCodeChangeReasonLoopKey))
	payload := loopinfra.BuildYaklangFullChangeEvent(eventOp, path, content, version, sourceAction, reason)

	emitYaklangCodeChangeEvent(loop, payload)
	log.Infof("yaklang_code_change delivered: path=%s op=%s version=%d bytes=%d", path, eventOp, version, len(content))
	loop.Set(yaklangEditorLastEmittedContentKey, content)
	loop.Set(yaklangEditorLastEmittedVersionKey, version)
	loop.Set(yaklangEditorSyncPendingLoopKey, false)
	loopinfra.ClearLoopYaklangDeliveryPatch(loop)
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

func emitYaklangCodeChangeEvent(loop *reactloops.ReActLoop, payload loopinfra.YaklangCodeChangeEvent) {
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
