package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
)

func inlineOrSpillAttachedText(label, content string, limit int, emitter *Emitter) (inline string, spillNote string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "(empty)", ""
	}
	if len(content) <= limit {
		return content, ""
	}

	filePath := consts.TempAIFileFast(fmt.Sprintf("attached-%s-*.txt", label), content)
	if filePath != "" && emitter != nil {
		_, _ = emitter.EmitPinFilename(filePath)
	}

	inline = content[:limit]
	spillNote = fmt.Sprintf(
		"%s length %d exceeds inline limit %d; full content saved to file: %s\nUse file-reading tools to load the complete content.",
		label, len(content), limit, filePath,
	)
	return inline, spillNote
}
