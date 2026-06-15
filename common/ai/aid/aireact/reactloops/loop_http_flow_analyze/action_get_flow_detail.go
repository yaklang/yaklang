package loop_http_flow_analyze

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var getHTTPFlowDetailAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"get_http_flow_detail",
		"Load a specific HTTP flow using id, hash, or hidden_index selectors and summarize its request/response content.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("id", aitool.WithParam_Description("Numeric flow id to load")),
			aitool.WithStringParam("hash", aitool.WithParam_Description("Flow hash to load when id is unknown")),
			aitool.WithStringParam("hidden_index", aitool.WithParam_Description("Hidden index for locating the flow")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("id") == 0 && action.GetString("hash") == "" && action.GetString("hidden_index") == "" {
				return utils.Error("id/hash/hidden_index must be provided to locate a flow")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			db := consts.GetGormProjectDatabase()
			if db == nil {
				operator.Fail("project database is not available for loading HTTP flow detail")
				return
			}

			locatorDesc := buildLocatorDesc(action)
			log.Infof("[get_http_flow_detail] loading flow: %s", locatorDesc)

			emitStatus(loop, "加载流详情中 / Loading Flow Detail...")

			var flow *schema.HTTPFlow
			var err error

			invoker := loop.GetInvoker()

			switch {
			case action.GetInt("id") > 0:
				flow, err = yakit.GetHTTPFlow(db, int64(action.GetInt("id")))
			case action.GetString("hash") != "":
				flow, err = yakit.GetHTTPFlowByHash(db, action.GetString("hash"))
			default:
				flow, err = yakit.GetHTTPFlowByHiddenIndex(db, action.GetString("hidden_index"))
			}

			if err != nil || flow == nil {
				emitStatus(loop, "加载失败：未找到流 / Load Failed: Flow Not Found")
				invoker.AddToTimeline("get_http_flow_detail", fmt.Sprintf("Failed to load HTTP flow: %v", err))
				log.Errorf("[get_http_flow_detail] failed to load (%s): %v", locatorDesc, err)
				recordAction(loop, "get_http_flow_detail", locatorDesc, "failed: flow not found", "")
				operator.Continue()
				return
			}

			emitStatus(loop, "加载完成 / Load Complete")

			req := flowRequest(flow)
			rsp := flowResponse(flow)

			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("Flow #%d hash=%s hidden_index=%s\n", flow.ID, flow.Hash, flow.HiddenIndex))
			builder.WriteString(fmt.Sprintf("Method: %s | Status: %d | URL: %s | Tags: %s | Source: %s | Runtime: %s\n\n",
				flow.Method, flow.StatusCode, utils.ShrinkString(flow.Url, 200), shrinkTags(flow.Tags), flow.SourceType, flow.RuntimeId))

			if req != "" {
				builder.WriteString("=== Request ===\n")
				builder.WriteString(utils.ShrinkTextBlock(req, 1200))
				builder.WriteString("\n\n")
			} else {
				builder.WriteString("=== Request ===\n(empty or oversized)\n\n")
			}

			if rsp != "" {
				builder.WriteString("=== Response ===\n")
				builder.WriteString(utils.ShrinkTextBlock(rsp, 1200))
				builder.WriteString("\n")
			} else {
				builder.WriteString("=== Response ===\n(empty or oversized)\n")
			}

			summary := builder.String()
			invoker.AddToTimeline("get_http_flow_detail", summary)
			loop.Set("current_flow", summary)

			// 输出简洁的累积流（2行）
			line1 := fmt.Sprintf("加载: %s", locatorDesc)

			reqSize := len(req)
			rspSize := len(rsp)
			tagsStr := ""
			if flow.Tags != "" {
				tagsStr = fmt.Sprintf(", tags=%s", shrinkTags(flow.Tags))
			}
			line2 := fmt.Sprintf("信息: %s %s %d %s (%s req, %s rsp)%s",
				flow.Method,
				utils.ShrinkString(flow.Url, 80),
				flow.StatusCode,
				http.StatusText(int(flow.StatusCode)),
				humanizeSize(reqSize),
				humanizeSize(rspSize),
				tagsStr)

			emitActionLog(loop, "http-flow-detail", line1, line2)

			flowBrief := fmt.Sprintf("#%d %s %d %s", flow.ID, flow.Method, flow.StatusCode, utils.ShrinkString(flow.Url, 80))
			log.Infof("[get_http_flow_detail] loaded: %s (req=%d bytes, rsp=%d bytes, tags=%s, source=%s)",
				flowBrief, len(req), len(rsp), shrinkTags(flow.Tags), flow.SourceType)
			recordAction(loop, "get_http_flow_detail", locatorDesc, flowBrief, "")

			operator.Feedback(summary)
		},
	)
}

func buildLocatorDesc(action *aicommon.Action) string {
	var parts []string
	if v := action.GetInt("id"); v > 0 {
		parts = append(parts, fmt.Sprintf("id=%d", v))
	}
	if v := action.GetString("hash"); v != "" {
		parts = append(parts, fmt.Sprintf("hash=%s", utils.ShrinkString(v, 20)))
	}
	if v := action.GetString("hidden_index"); v != "" {
		parts = append(parts, fmt.Sprintf("hidden_index=%s", utils.ShrinkString(v, 30)))
	}
	if len(parts) == 0 {
		return "(no locator)"
	}
	return strings.Join(parts, ", ")
}

func humanizeSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}
