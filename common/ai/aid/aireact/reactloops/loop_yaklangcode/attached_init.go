package loop_yaklangcode

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
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
	if !hasYaklangEditorDeliveryTarget(loop) {
		seedYaklangLoopFullCode(loop, editorCtx, "")
		log.Infof("create mode: no editor target file; delivery path deferred until loop flush")
		operator.Continue()
		return
	}

	targetPath, fromAttached := aicommon.ResolveYaklangInitTargetPath(editorCtx, liteforgePath)
	if targetPath == "" {
		seedYaklangLoopFullCode(loop, editorCtx, "")
		log.Infof("create mode: no resolvable target path; delivery deferred until loop flush")
		operator.Continue()
		return
	}

	_, hasAttachedCode := aicommon.YaklangAttachedInitialCode(editorCtx)
	log.Infof("edit mode: target path %s (attached=%v, attached_code=%v)", targetPath, fromAttached, hasAttachedCode)
	diskContent := ""
	if content, readErr := os.ReadFile(targetPath); readErr == nil {
		diskContent = string(content)
	} else if readErr != nil {
		log.Warnf("edit mode: failed to read target file %s: %v", targetPath, readErr)
	}
	seedYaklangLoopFullCode(loop, editorCtx, diskContent)

	ensureYaklangLoopStagingFilename(loop, r)
	operator.Continue()
}

func seedYaklangLoopFullCode(loop *reactloops.ReActLoop, editorCtx *aicommon.YaklangEditorContext, diskContent string) {
	seedCode, fromSelection := aicommon.ResolveYaklangInitFullCode(editorCtx, diskContent)
	if strings.TrimSpace(seedCode) == "" {
		return
	}
	lineBase := aicommon.YaklangCodeLineBase(editorCtx, fromSelection)
	loop.Set("full_code", seedCode)
	loop.Set(loopinfra.LoopVarCodeLineBase, lineBase)
	log.Infof("seeded full_code (%d bytes, from_selection=%v, code_line_base=%d)", len(seedCode), fromSelection, lineBase)
}
