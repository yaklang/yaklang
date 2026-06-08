package loop_yaklangcode

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// yaklangFilePathFromAttached resolves the target Yak file path from frontend attached resources.
func yaklangFilePathFromAttached(attachedDatas []*aicommon.AttachedResource) string {
	ctx := yaklangEditorContextFromAttached(attachedDatas)
	if ctx == nil {
		return ""
	}
	return ctx.EditorFile
}
