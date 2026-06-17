package loop_yaklangcode

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const yaklangCodeStagingFilenameLoopKey = "yaklang_code_staging_filename"

// ensureYaklangLoopStagingFilename binds loop.filename to an aispace staging file.
// User-facing delivery paths live in editor_file_path; intermediate write/modify actions
// must not touch the editor file on disk until deferred editor sync flushes.
func ensureYaklangLoopStagingFilename(loop *reactloops.ReActLoop, runtime aicommon.AIInvokeRuntime) string {
	if loop == nil || runtime == nil {
		return ""
	}
	if staging := strings.TrimSpace(loop.Get(yaklangCodeStagingFilenameLoopKey)); staging != "" {
		loop.Set("filename", staging)
		return staging
	}
	staging := strings.TrimSpace(runtime.EmitFileArtifactWithExt("yaklang_code_staging", ".yak", ""))
	if staging == "" {
		return ""
	}
	loop.Set(yaklangCodeStagingFilenameLoopKey, staging)
	loop.Set("filename", staging)
	return staging
}
