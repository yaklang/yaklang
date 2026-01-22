package loop_http_flow_analyze

import (
	"fmt"
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

			var flow *schema.HTTPFlow
			var err error

			switch {
			case action.GetInt("id") > 0:
				flow, err = yakit.GetHTTPFlow(db, int64(action.GetInt("id")))
			case action.GetString("hash") != "":
				flow, err = yakit.GetHTTPFlowByHash(db, action.GetString("hash"))
			default:
				flow, err = yakit.GetHTTPFlowByHiddenIndex(db, action.GetString("hidden_index"))
			}

			if err != nil || flow == nil {
				log.Errorf("get_http_flow_detail failed: %v", err)
				operator.Fail(fmt.Sprintf("failed to load http flow: %v", err))
				return
			}

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
			loop.GetInvoker().AddToTimeline("get_http_flow_detail", summary)
			loop.Set("current_flow", summary)

			operator.Feedback(summary)
		},
	)
}
