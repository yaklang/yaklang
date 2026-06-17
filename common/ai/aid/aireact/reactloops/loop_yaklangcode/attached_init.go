package loop_yaklangcode

import (
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// initYaklangEditorContextFromAttached parses attachments, binds loop keys, and records timeline.
func initYaklangEditorContextFromAttached(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	attachedDatas []*aicommon.AttachedResource,
) *aicommon.YaklangEditorContext {
	ctx := aicommon.ParseYaklangEditorContextFromAttached(attachedDatas)
	if ctx == nil {
		return nil
	}

	if ctx.HasWorkspace() {
		loop.Set("workspace_path", ctx.WorkspacePath)
	}
	if ctx.HasEditorFile() {
		loop.Set("editor_file_path", ctx.EditorFile)
	}

	payload := aicommon.FormatYaklangEditorContextMarkdown(ctx)
	if payload != "" {
		r.AddToTimeline("yaklang_editor_context", payload)
		r.AddToTimeline(
			"import notice",
			"yaklang_editor_context records the user's open workspace and file; prefer these paths over guessed paths.",
		)
	}
	return ctx
}

// finalizeYaklangInitFileTarget seeds full_code and loop.filename for the yaklang init step.
// Edit mode (editor_file_path set): read disk or attached selection, bind aispace staging filename.
// Create mode (no editor target): seed selection-only content if present; delivery path deferred to flush.
func finalizeYaklangInitFileTarget(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	emitter *aicommon.Emitter,
	operator *reactloops.InitTaskOperator,
	editorCtx *aicommon.YaklangEditorContext,
	liteforgePath string,
) {
	_ = emitter
	attachedCode, hasAttachedCode := aicommon.YaklangAttachedInitialCode(editorCtx)

	if !hasYaklangEditorDeliveryTarget(loop) {
		if hasAttachedCode {
			loop.Set("full_code", attachedCode)
			log.Infof("create mode: seeded full_code from attached selection, size: %d bytes", len(attachedCode))
		}
		log.Infof("create mode: no editor target file; delivery path deferred until loop flush")
		operator.Continue()
		return
	}

	targetPath, fromAttached := aicommon.ResolveYaklangInitTargetPath(editorCtx, liteforgePath)
	if targetPath == "" {
		if hasAttachedCode {
			loop.Set("full_code", attachedCode)
		}
		log.Infof("create mode: no resolvable target path; delivery deferred until loop flush")
		operator.Continue()
		return
	}

	log.Infof("edit mode: target path %s (attached=%v, attached_code=%v)", targetPath, fromAttached, hasAttachedCode)
	if hasAttachedCode {
		loop.Set("full_code", attachedCode)
		log.Infof("seeded full_code from attached selection, size: %d bytes", len(attachedCode))
	} else if content, readErr := os.ReadFile(targetPath); readErr == nil && len(content) > 0 {
		seedCode, _ := aicommon.ResolveYaklangInitFullCode(editorCtx, string(content))
		log.Infof("seeded full_code from disk file %s, size: %d bytes", targetPath, len(content))
		loop.Set("full_code", seedCode)
	}

	ensureYaklangLoopStagingFilename(loop, r)
	operator.Continue()
}
