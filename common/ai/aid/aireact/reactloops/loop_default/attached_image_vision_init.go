package loop_default

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

func collectAttachedImagePaths(attachedDatas []*aicommon.AttachedResource) []string {
	var paths []string
	for _, data := range attachedDatas {
		if data == nil {
			continue
		}
		if data.Type != aicommon.CONTEXT_PROVIDER_TYPE_FILE || data.Key != aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH {
			continue
		}
		p := strings.TrimSpace(data.Value)
		if p == "" || !aicommon.IsImageContextAttachmentPath(p) {
			continue
		}
		paths = append(paths, p)
	}
	return dedupStringSlice(paths)
}

func dedupStringSlice(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// RunAttachedImageVisionInDefaultInit runs LiteForge vision on each attached image path
// and injects the aggregated text into the invoker timeline for the main ReAct loop.
func RunAttachedImageVisionInDefaultInit(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	imagePaths []string,
) {
	if len(imagePaths) == 0 || r == nil {
		return
	}

	userQuery := strings.TrimSpace(task.GetUserInput())
	extra := fmt.Sprintf(`
**Supplementary Information (User task / question — must be integrated into your analysis):**
%s

In cumulative_summary, besides an objective description of the image, explicitly state:
(1) visible facts that directly relate to the user's task,
(2) clues that may help answer it,
(3) anything that cannot be determined from the image alone and may need user clarification.
`, userQuery)

	runCtx := loop.GetConfig().GetContext()
	if task != nil && !utils.IsNil(task.GetContext()) {
		runCtx = task.GetContext()
	}
	if utils.IsNil(runCtx) {
		runCtx = context.Background()
	}

	var buf strings.Builder
	buf.WriteString("## 附件图片 — 视觉解析（LiteForge / TierVision）\n\n")

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

		log.Infof("loop_default: vision analyze attached image %q (%d/%d)", imagePath, i+1, len(imagePaths))
		analysis, err := aiforge.AnalyzeImageFile(imagePath,
			aiforge.WithAnalyzeContext(runCtx),
			aiforge.WithExtraPrompt(extra),
			aiforge.WithAnalyzeStatusCard(statusCb),
		)

		buf.WriteString(fmt.Sprintf("### %s\n\n", imagePath))
		if err != nil {
			buf.WriteString(fmt.Sprintf("_视觉解析失败 / vision analysis failed: %v_\n\n", err))
			log.Warnf("loop_default: vision failed for %q: %v", imagePath, err)
			continue
		}
		if analysis == nil {
			buf.WriteString("_视觉解析返回空结果 / empty analysis_\n\n")
			continue
		}
		if s := strings.TrimSpace(analysis.CumulativeSummary); s != "" {
			buf.WriteString("**总体摘要（含与用户问题的关联）**\n\n")
			buf.WriteString(s)
			buf.WriteString("\n\n")
		}
		if detail := strings.TrimSpace(analysis.Dump()); detail != "" {
			buf.WriteString("**结构化细节**\n\n```\n")
			buf.WriteString(detail)
			buf.WriteString("\n```\n\n")
		}
		if ocr := strings.TrimSpace(analysis.OCR()); ocr != "" {
			buf.WriteString("**OCR 文本**\n\n```\n")
			buf.WriteString(ocr)
			buf.WriteString("\n```\n\n")
		}
	}

	loop.LoadingStatus("附件图片解析完成 / Finished parsing attached images")

	payload := strings.TrimSpace(buf.String())
	if payload == "" {
		return
	}
	r.AddToTimeline("attached_image_vision", payload)
	r.AddToTimeline("import notice", "attached_image_vision has been recorded from default-loop init; use it when reasoning about the user's images.")
}
