package reactloops

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	DefaultSpillThresholdBytes = 5 * 1024
	DefaultSpillPreviewBytes   = 500
	defaultSpillDataSubdir     = "data"
)

func spillFilename(loop *ReActLoop, prefix string) string {
	if loop == nil {
		return ""
	}
	dataDir := loop.GetLoopContentDir(defaultSpillDataSubdir)
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir,
		fmt.Sprintf("%s_%d_%s.txt", prefix, loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))
}

// SaveSpillContent writes content to the loop data directory and pins the file.
// Returns filename and a short preview. Empty filename means save was not performed or failed.
func SaveSpillContent(loop *ReActLoop, prefix, content string) (filename string, preview string) {
	if loop == nil || content == "" {
		return "", ""
	}
	preview = utils.ShrinkTextBlock(content, DefaultSpillPreviewBytes)
	filename = spillFilename(loop, prefix)
	if filename == "" {
		return "", preview
	}
	if err := SaveAndPinFile(loop, filename, []byte(content)); err != nil {
		log.Warnf("SaveSpillContent: failed to save %s: %v", prefix, err)
		return "", preview
	}
	return filename, preview
}

// SpillLongContent returns inline content when within threshold; otherwise spills to file
// and returns a user-facing summary plus reference material for ActionLog.
func SpillLongContent(loop *ReActLoop, prefix, content string) (summary string, reference string) {
	if content == "" {
		return "", ""
	}
	if len(content) <= DefaultSpillThresholdBytes {
		return content, content
	}
	filename, preview := SaveSpillContent(loop, prefix, content)
	if filename == "" {
		summary = fmt.Sprintf("结果过长，预览:\n%s", preview)
		return summary, summary
	}
	summary = fmt.Sprintf("结果过长 (%d bytes)，已保存到文件。\n\n预览:\n%s\n\n文件: %s",
		len(content), preview, filename)
	return summary, summary
}

// SaveContentReference always returns a preview; spills to file when content exceeds previewBytes.
func SaveContentReference(loop *ReActLoop, prefix, content string, previewBytes int) (filename string, preview string) {
	content = strings.TrimSpace(content)
	if content == "" || loop == nil {
		return "", ""
	}
	if previewBytes <= 0 {
		previewBytes = 1200
	}
	preview = utils.ShrinkTextBlock(content, previewBytes)
	if len(content) <= previewBytes {
		return "", preview
	}
	filename = spillFilename(loop, prefix)
	if filename == "" {
		return "", preview
	}
	if err := SaveAndPinFile(loop, filename, []byte(content)); err != nil {
		log.Warnf("SaveContentReference: failed to save %s: %v", prefix, err)
		return "", preview
	}
	return filename, preview
}
