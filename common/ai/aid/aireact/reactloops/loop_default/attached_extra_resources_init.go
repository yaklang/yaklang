package loop_default

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

// RunAttachedExtraResourcesInDefaultInit renders HTTP flow and selected-text attachments
// into the invoker timeline for the main ReAct loop.
func RunAttachedExtraResourcesInDefaultInit(
	r aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	attachedDatas []*aicommon.AttachedResource,
) {
	if len(attachedDatas) == 0 || r == nil {
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
				log.Warnf("loop_default: %s", msg)
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
		r.AddToTimeline("import notice", "attached_http_flow has been recorded from default-loop init; use it when reasoning about the user's HTTP traffic.")
	}

	if len(selectedSections) > 0 {
		payload := strings.Join(selectedSections, "\n\n---\n\n")
		r.AddToTimeline("attached_selected_text", payload)
		r.AddToTimeline("import notice", "attached_selected_text has been recorded from default-loop init; use it when reasoning about the user's UI selection.")
	}

	if len(httpFlowSections) > 0 || len(selectedSections) > 0 {
		loop.LoadingStatus("附加 HTTP 流量与用户选中文本处理完成 / Finished loading attached HTTP flows and selected text")
	}
}
