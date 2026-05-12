package loop_knowledge_enhance

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func splitAttachedImagePaths(paths []string) (images, others []string) {
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if aicommon.IsImageContextAttachmentPath(p) {
			images = append(images, p)
		} else {
			others = append(others, p)
		}
	}
	return images, others
}

// appendImageVisionToAttachedResources runs LiteForge vision on each image path,
// emits loading/status via the loop, and appends markdown to resourcesInfo for the enhance prompt.
func appendImageVisionToAttachedResources(
	loop *reactloops.ReActLoop,
	ctx context.Context,
	userQuery string,
	imagePaths []string,
	resourcesInfo *strings.Builder,
) {
	if len(imagePaths) == 0 || resourcesInfo == nil {
		return
	}
	if utils.IsNil(ctx) {
		ctx = context.Background()
	}

	userQuery = strings.TrimSpace(userQuery)
	extra := fmt.Sprintf(`
**Supplementary Information (User task / question — must be integrated into your analysis):**
%s

In cumulative_summary, besides an objective description of the image, explicitly state:
(1) visible facts that directly relate to the user's task,
(2) clues that may help answer it,
(3) anything that cannot be determined from the image alone and may need user clarification.
`, userQuery)

	resourcesInfo.WriteString("### 附件图片 — 视觉解析（LiteForge）\n\n")

	for i, imagePath := range imagePaths {
		base := filepath.Base(imagePath)
		loop.LoadingStatus(fmt.Sprintf("正在解析附件图片 (%d/%d): %s / Parsing attached image: %s", i+1, len(imagePaths), base, base))

		statusCb := func(id string, data interface{}, tags ...string) {
			msg := fmt.Sprintf("图片解析进度 / Image analysis [%s]: %v", id, data)
			if len(tags) > 0 {
				msg = fmt.Sprintf("%s tags=%v", msg, tags)
			}
			loop.LoadingStatus(msg)
		}

		log.Infof("knowledge enhance: vision analyze attached image %q (%d/%d)", imagePath, i+1, len(imagePaths))
		analysis, err := aiforge.AnalyzeImageFile(imagePath,
			aiforge.WithAnalyzeContext(ctx),
			aiforge.WithExtraPrompt(extra),
			aiforge.WithAnalyzeStatusCard(statusCb),
		)

		resourcesInfo.WriteString(fmt.Sprintf("#### %s\n\n", imagePath))
		if err != nil {
			resourcesInfo.WriteString(fmt.Sprintf("_视觉解析失败 / vision analysis failed: %v_\n\n", err))
			log.Warnf("knowledge enhance: vision failed for %q: %v", imagePath, err)
			continue
		}
		if analysis == nil {
			resourcesInfo.WriteString("_视觉解析返回空结果 / empty analysis_\n\n")
			continue
		}
		if s := strings.TrimSpace(analysis.CumulativeSummary); s != "" {
			resourcesInfo.WriteString("**总体摘要（含与用户问题的关联）**\n\n")
			resourcesInfo.WriteString(s)
			resourcesInfo.WriteString("\n\n")
		}
		if detail := strings.TrimSpace(analysis.Dump()); detail != "" {
			resourcesInfo.WriteString("**结构化细节**\n\n```\n")
			resourcesInfo.WriteString(detail)
			resourcesInfo.WriteString("\n```\n\n")
		}
		if ocr := strings.TrimSpace(analysis.OCR()); ocr != "" {
			resourcesInfo.WriteString("**OCR 文本**\n\n```\n")
			resourcesInfo.WriteString(ocr)
			resourcesInfo.WriteString("\n```\n\n")
		}
	}

	loop.LoadingStatus("附件图片解析完成 / Finished parsing attached images")
}
