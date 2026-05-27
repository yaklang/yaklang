package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// RunAttachedExtraResourcesInit renders http_flow_id and selected attachments into the
// invoker timeline so specialized loops (default, http flow analyze, http fuzztest, etc.)
// can consume the same attached-resource payload.
func RunAttachedExtraResourcesInit(
	r aicommon.AIInvokeRuntime,
	loop *ReActLoop,
	attachedDatas []*aicommon.AttachedResource,
) {
	if len(attachedDatas) == 0 || r == nil || loop == nil {
		return
	}

	var httpFlowSections []string
	var selectedSections []string
	db := consts.GetGormProjectDatabase()
	emitter := loop.GetEmitter()

	for idx, data := range attachedDatas {
		if data == nil {
			continue
		}
		switch {
		case aicommon.IsAttachedHTTPFlowResource(data):
			loop.LoadingStatus(fmt.Sprintf(
				"正在加载附加 HTTP 流量 (%d) / Loading attached HTTP flow (%d)",
				idx+1, idx+1,
			))
			rendered, err := aicommon.RenderAttachedHTTPFlowResource(db, data, emitter)
			if err != nil {
				msg := fmt.Sprintf("failed to load attached HTTP flow %q: %v", strings.TrimSpace(data.Value), err)
				log.Warnf("attached extra resources: %s", msg)
				httpFlowSections = append(httpFlowSections, fmt.Sprintf("## Attached HTTP Flow\n\n_Error: %s_", msg))
				continue
			}
			httpFlowSections = append(httpFlowSections, rendered)
		case aicommon.IsAttachedSelectedResource(data):
			loop.LoadingStatus(fmt.Sprintf(
				"正在加载用户选中文本 (%d) / Loading attached selected text (%d)",
				idx+1, idx+1,
			))
			selectedSections = append(selectedSections, aicommon.RenderAttachedSelectedResource(data, emitter))
		}
	}

	if len(httpFlowSections) > 0 {
		payload := strings.Join(httpFlowSections, "\n\n---\n\n")
		r.AddToTimeline("attached_http_flow", payload)
		r.AddToTimeline("import notice", "attached_http_flow has been recorded; use it when reasoning about the user's HTTP traffic.")
	}

	if len(selectedSections) > 0 {
		payload := strings.Join(selectedSections, "\n\n---\n\n")
		r.AddToTimeline("attached_selected_text", payload)
		r.AddToTimeline("import notice", "attached_selected_text has been recorded; use it when reasoning about the user's UI selection.")
	}

	if len(httpFlowSections) > 0 || len(selectedSections) > 0 {
		loop.LoadingStatus("附加 HTTP 流量与用户选中文本处理完成 / Finished loading attached HTTP flows and selected text")
	}
}
