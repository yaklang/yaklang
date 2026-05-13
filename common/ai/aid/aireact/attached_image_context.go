package aireact

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// AttachedImageVisionContextProvider returns a ContextProvider that runs LiteForge
// vision analysis once, caches the text, and injects structured image understanding
// plus linkage to the user's question into the prompt context.
func AttachedImageVisionContextProvider(filePath string, userPrompt ...string) aicommon.ContextProvider {
	userTask := strings.TrimSpace(strings.Join(userPrompt, " "))
	extra := fmt.Sprintf(`
**Supplementary Information (User task / question — must be integrated into your analysis):**
%s

In cumulative_summary, besides an objective description of the image, explicitly state:
(1) visible facts that directly relate to the user's task,
(2) clues that may help answer it,
(3) anything that cannot be determined from the image alone and may need user clarification.
`, userTask)

	var once sync.Once
	var cached string
	var cachedErr error

	return func(config aicommon.AICallerConfigIf, _ *aicommon.Emitter, _ string) (string, error) {
		once.Do(func() {
			base := fmt.Sprintf("User Prompt: %s\nFile: %s\n", userTask, filePath)
			if !utils.FileExists(filePath) {
				cached = base + "[Error: image file does not exist]"
				cachedErr = utils.Errorf("file %s does not exist", filePath)
				return
			}

			ctx := config.GetContext()
			if utils.IsNil(ctx) {
				ctx = context.Background()
			}

			log.Infof("aireact: vision liteforge analyze for attached image: %s", filePath)
			analysis, err := aiforge.AnalyzeImageFile(filePath,
				aiforge.WithAnalyzeContext(ctx),
				aiforge.WithExtraPrompt(extra),
			)
			if err != nil {
				cached = base + fmt.Sprintf("\n[视觉解析失败 / vision analysis failed: %v]\n", err)
				cachedErr = nil
				return
			}

			var b strings.Builder
			b.WriteString(base)
			b.WriteString("\n## 附加图片 — 视觉解析（LiteForge）\n\n")
			b.WriteString("以下为视觉模型根据图像与用户问题提取的信息，请在后续推理中优先参考。\n\n")
			if analysis != nil && strings.TrimSpace(analysis.CumulativeSummary) != "" {
				b.WriteString("### 总体摘要（含与用户问题的关联）\n")
				b.WriteString(analysis.CumulativeSummary)
				b.WriteString("\n\n")
			}
			if analysis != nil {
				detail := strings.TrimSpace(analysis.Dump())
				if detail != "" {
					b.WriteString("### 结构化细节（元素 / 文本 / 关系 / 场景）\n")
					b.WriteString(detail)
					b.WriteString("\n\n")
				}
				ocr := strings.TrimSpace(analysis.OCR())
				if ocr != "" {
					b.WriteString("### OCR 文本汇总\n")
					b.WriteString(ocr)
					b.WriteString("\n")
				}
			}
			cached = b.String()
			cachedErr = nil
		})
		return cached, cachedErr
	}
}
