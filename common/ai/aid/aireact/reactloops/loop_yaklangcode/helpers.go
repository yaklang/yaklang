package loop_yaklangcode

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const maxYaklangOutputSummaryBytes = 5 * 1024

func spillYaklangContent(loop *reactloops.ReActLoop, prefix string, content string) (summary string, reference string) {
	if content == "" {
		return "", ""
	}
	if len(content) <= maxYaklangOutputSummaryBytes {
		return content, content
	}
	loopDataDir := loop.GetLoopContentDir("data")
	filename := filepath.Join(loopDataDir,
		fmt.Sprintf("%s_%d_%s.txt", prefix, loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))
	if err := reactloops.SaveAndPinFile(loop, filename, []byte(content)); err != nil {
		log.Warnf("[loop_yaklangcode] failed to save spill content %s: %v", prefix, err)
		preview := utils.ShrinkTextBlock(content, 500)
		summary = fmt.Sprintf("结果过长，预览:\n%s", preview)
		return summary, summary
	}
	preview := utils.ShrinkTextBlock(content, 500)
	summary = fmt.Sprintf("结果过长 (%d bytes)，已保存到文件。\n\n预览:\n%s\n\n文件: %s",
		len(content), preview, filename)
	return summary, summary
}
