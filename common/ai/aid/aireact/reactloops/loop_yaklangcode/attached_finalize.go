package loop_yaklangcode

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// attachedInitialCode returns editor-attached code that should seed loop full_code.
// Selection content from the frontend takes precedence over reading the file from disk.
func attachedInitialCode(ctx *YaklangEditorContext) (code string, ok bool) {
	if ctx == nil || !ctx.HasSelection() {
		return "", false
	}
	code = strings.TrimSpace(ctx.Selection.Content)
	return code, code != ""
}

// resolveYaklangInitFullCode prefers attached selection content over on-disk file bytes.
func resolveYaklangInitFullCode(editorCtx *YaklangEditorContext, diskContent string) (code string, fromAttached bool) {
	if attachedCode, ok := attachedInitialCode(editorCtx); ok {
		return attachedCode, true
	}
	return diskContent, false
}

func resolveYaklangInitTargetPath(editorCtx *YaklangEditorContext, liteforgePath string) (targetPath string, fromAttached bool) {
	if editorCtx != nil && editorCtx.HasEditorFile() {
		return editorCtx.EditorFile, true
	}
	liteforgePath = strings.TrimSpace(liteforgePath)
	if liteforgePath != "" {
		return liteforgePath, false
	}
	return "", false
}

// finalizeYaklangInitFileTarget applies Step-3 file targeting for yaklang init.
// When the frontend attached a code selection, full_code is seeded from the attachment
// instead of os.ReadFile so the loop edits the user's in-editor context.
func finalizeYaklangInitFileTarget(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	emitter *aicommon.Emitter,
	operator *reactloops.InitTaskOperator,
	editorCtx *YaklangEditorContext,
	liteforgePath string,
) {
	targetPath, fromAttached := resolveYaklangInitTargetPath(editorCtx, liteforgePath)
	attachedCode, hasAttachedCode := attachedInitialCode(editorCtx)

	if targetPath != "" {
		log.Infof("identified target path: %s (attached=%v, attached_code=%v)", targetPath, fromAttached, hasAttachedCode)
		filename := utils.GetFirstExistedFile(targetPath)
		if filename == "" {
			// Target file may not exist yet; keep edits in loop memory until the user accepts in the editor.
			filename = targetPath
		}

		if hasAttachedCode {
			loop.Set("full_code", attachedCode)
			log.Infof("seeded full_code from attached selection, size: %d bytes", len(attachedCode))
		} else if content, readErr := os.ReadFile(targetPath); readErr == nil && len(content) > 0 {
			seedCode, _ := resolveYaklangInitFullCode(editorCtx, string(content))
			log.Infof("identified target file: %s, file size: %v", targetPath, len(content))
			loop.Set("full_code", seedCode)
		}

		emitter.EmitPinFilename(filename)
		loop.Set("filename", filename)
		operator.Continue()
		return
	}

	filename := r.EmitFileArtifactWithExt("gen_code", ".yak", "")
	if hasAttachedCode {
		loop.Set("full_code", attachedCode)
		log.Infof("seeded new artifact full_code from attached selection, size: %d bytes", len(attachedCode))
	}
	emitter.EmitPinFilename(filename)
	loop.Set("filename", filename)
	operator.Continue()
}
