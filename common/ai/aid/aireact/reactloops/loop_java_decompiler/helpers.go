package loop_java_decompiler

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const maxJavaDecompilerSummaryBytes = 5 * 1024

func saveSpillContent(loop *reactloops.ReActLoop, prefix string, content string) (filename string, preview string) {
	if loop == nil || content == "" {
		return "", ""
	}
	loopDataDir := loop.GetLoopContentDir("data")
	filename = filepath.Join(loopDataDir,
		fmt.Sprintf("%s_%d_%s.txt", prefix, loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))
	if err := reactloops.SaveAndPinFile(loop, filename, []byte(content)); err != nil {
		log.Warnf("[java_decompiler] failed to save spill content %s: %v", prefix, err)
		return "", utils.ShrinkTextBlock(content, 500)
	}
	return filename, utils.ShrinkTextBlock(content, 500)
}

func spillOrPreview(loop *reactloops.ReActLoop, prefix string, content string) (summary string, reference string) {
	if content == "" {
		return "", ""
	}
	if len(content) <= maxJavaDecompilerSummaryBytes {
		return content, content
	}
	filename, preview := saveSpillContent(loop, prefix, content)
	summary = fmt.Sprintf("结果过长 (%d bytes)，已保存到文件。\n\n预览:\n%s\n\n文件: %s",
		len(content), preview, filename)
	return summary, summary
}
